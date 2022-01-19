//
// Copyright (C) 2019-2022 vdaas.org vald team <vald@vdaas.org>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

// Package service manages the main logic of server.
package service

import (
	"context"
	"encoding/gob"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vdaas/vald/internal/config"
	core "github.com/vdaas/vald/internal/core/algorithm/faiss"
	"github.com/vdaas/vald/internal/errgroup"
	"github.com/vdaas/vald/internal/errors"
	"github.com/vdaas/vald/internal/file"
	"github.com/vdaas/vald/internal/log"
	"github.com/vdaas/vald/internal/observability/trace"
	"github.com/vdaas/vald/internal/safety"
	"github.com/vdaas/vald/internal/strings"
	"github.com/vdaas/vald/pkg/agent/core/faiss/model"
	"github.com/vdaas/vald/pkg/agent/core/ngt/service/kvs"
	"github.com/vdaas/vald/pkg/agent/core/ngt/service/vqueue"
	"github.com/vdaas/vald/pkg/agent/internal/metadata"
)

type (
	Faiss interface {
		Start(ctx context.Context) <-chan error
		Train(nb int, xb []float32) error
		Insert(uuid string, xb []float32) error
		InsertWithTime(uuid string, vec []float32, t int64) error
		CreateIndex(ctx context.Context) error
		SaveIndex(ctx context.Context) error
		CreateAndSaveIndex(ctx context.Context) error
		Search(k, nq uint32, xq []float32) ([]model.Distance, error)
		Delete(uuid string) error
		DeleteWithTime(uuid string, t int64) error
		Exists(uuid string) (uint32, bool)
		IsIndexing() bool
		IsSaving() bool
		NumberOfProactiveGCExecution() uint64
		Len() uint64
		GetDimensionSize() int
		Close(ctx context.Context) error
	}

	faiss struct {
		core core.Faiss
		eg   errgroup.Group
		kvs  kvs.BidiMap
		fmu  sync.Mutex
		fmap map[string]uint32 //failure map for index
		vq   vqueue.Queue
		icnt uint64

		// statuses
		indexing  atomic.Value
		saving    atomic.Value
		cimu      sync.Mutex // create index mutex
		lastNocie uint64     // last number of create index execution this value prevent unnecessary saveindex

		// counters
		nocie uint64 // number of create index execution
		nogce uint64 // number of proactive GC execution

		// configurations
		inMem             bool          // in-memory mode
		dim               int           // dimension size
		alen              int           // auto indexing length
		dur               time.Duration // auto indexing check duration
		sdur              time.Duration // auto save index check duration
		lim               time.Duration // auto indexing time limit
		minLit            time.Duration // minimum load index timeout
		maxLit            time.Duration // maximum load index timeout
		litFactor         time.Duration // load index timeout factor
		enableProactiveGC bool          // if this value is true, agent component will purge GC memory more proactive
		enableCopyOnWrite bool          // if this value is true, agent component will write backup file using Copy on Write and saves old files to the old directory
		path              string        // index path
		smu               sync.Mutex    // save index lock
		tmpPath           atomic.Value  // temporary index path for Copy on Write
		oldPath           string        // old volume path
		basePath          string        // index base directory for CoW
		cowmu             sync.Mutex    // copy on write move lock
		dcd               bool          // disable commit daemon
		idelay            time.Duration // initial delay duration
		kvsdbConcurrency  int           // kvsdb concurrency
	}
)

const (
	kvsFileName = "faiss-meta.kvsdb"

	oldIndexDirName    = "backup"
	originIndexDirName = "origin"
)

func New(cfg *config.Faiss, opts ...Option) (Faiss, error) {
	var (
		f = &faiss{
			fmap:              make(map[string]uint32),
			dim:               cfg.Dimension,
			enableProactiveGC: cfg.EnableProactiveGC,
			enableCopyOnWrite: cfg.EnableCopyOnWrite,
			kvsdbConcurrency:  cfg.KVSDB.Concurrency,

		}
		err error
	)

	for _, opt := range append(defaultOptions, opts...) {
		if err := opt(f); err != nil {
			// TODO errors/faiss.go
			return nil, errors.ErrOptionFailed(err, reflect.ValueOf(opt))
		}
	}

	if len(f.path) == 0 {
		f.inMem = true
	}

	if f.enableCopyOnWrite && !f.inMem && len(f.path) != 0 {
		sep := string(os.PathSeparator)
		f.path, err = filepath.Abs(strings.ReplaceAll(f.path, sep+sep, sep))
		if err != nil {
			log.Warn(err)
		}

		f.basePath = f.path
		f.oldPath = file.Join(f.basePath, oldIndexDirName)
		f.path = file.Join(f.basePath, originIndexDirName)
		err = file.MkdirAll(f.oldPath, fs.ModePerm)
		if err != nil {
			log.Warn(err)
		}
		err = file.MkdirAll(f.path, fs.ModePerm)
		if err != nil {
			log.Warn(err)
		}
		err = f.mktmp()
		if err != nil {
			return nil, err
		}
	}

	err = f.initFaiss(
		core.WithDimension(cfg.Dimension),
		core.WithNlist(cfg.Nlist),
		core.WithM(cfg.M),
		core.WithNbitsPerIdx(cfg.NbitsPerIdx),
	)
	if err != nil {
		return nil, err
	}

	if f.dur == 0 || f.alen == 0 {
		f.dcd = true
	}

	if f.vq == nil {
		f.vq, err = vqueue.New(
			vqueue.WithErrGroup(f.eg),
			vqueue.WithInsertBufferPoolSize(cfg.VQueue.InsertBufferPoolSize),
			vqueue.WithDeleteBufferPoolSize(cfg.VQueue.DeleteBufferPoolSize),
		)
		if err != nil {
			return nil, err
		}
	}

	f.indexing.Store(false)
	f.saving.Store(false)

	return f, nil
}

func (f *faiss) initFaiss(opts ...core.Option) error {
	var err error

	if f.kvs == nil {
		f.kvs = kvs.New(kvs.WithConcurrency(f.kvsdbConcurrency))
	}

	if f.inMem {
		log.Debug("vald agent starts with in-memory mode")
		f.core, err = core.New(opts...)
		return err
	}

	ctx := context.Background()
	err = f.load(ctx, f.path, opts...)
	var current uint64
	if err != nil {
		if !f.enableCopyOnWrite {
			log.Debug("failed to load vald index from %s\t error: %v", f.path, err)
			if f.kvs == nil {
				f.kvs = kvs.New(kvs.WithConcurrency(f.kvsdbConcurrency))
			} else if f.kvs.Len() > 0 {
				f.kvs.Close()
				f.kvs = kvs.New(kvs.WithConcurrency(f.kvsdbConcurrency))
			}

			if f.core != nil {
				f.core.Close()
				f.core = nil
			}
			f.core, err = core.New(append(opts, core.WithIndexPath(f.path))...)
			return err
		}

		// TODO errors/faiss.go
		if errors.Is(err, errors.ErrIndicesAreTooFewComparedToMetadata) && f.kvs != nil {
			current = f.kvs.Len()
			log.Warnf(
				"load vald primary index success from %s\t error: %v\tbut index data are too few %d compared to metadata count now trying to load from old copied index data from %s and compare them",
				f.path,
				err,
				current,
				f.oldPath,
			)
		} else {
			log.Warnf("failed to load vald primary index from %s\t error: %v\ttrying to load from old copied index data from %s", f.path, err, f.oldPath)
		}
	} else {
		return nil
	}

	err = f.load(ctx, f.oldPath, opts...)
	if err == nil {
		if current != 0 && f.kvs.Len() < current {
			log.Warnf(
				"load vald secondary index success from %s\t error: %v\tbut index data are too few %d compared to primary data now trying to load from primary index data again from %s and start up with them",
				f.oldPath,
				err,
				f.kvs.Len(),
				f.oldPath,
			)

			err = f.load(ctx, f.path, opts...)
			if err == nil {
				return nil
			}
		} else {
			return nil
		}
	}

	log.Warnf("failed to load vald secondary index from %s and %s\t error: %v\ttrying to load from non-CoW index data from %s for backwards compatibility", f.path, f.oldPath, err, f.basePath)
	err = f.load(ctx, f.basePath, opts...)
	if err == nil {
		file.CopyDir(ctx, f.basePath, f.path)
		return nil
	}

	tpath := f.tmpPath.Load().(string)
	log.Warnf(
		"failed to load vald backwards index from %s and %s and %s\t error: %v\tvald agent couldn't find any index from agent volume in %s trying to start as new index from %s",
		f.path,
		f.oldPath,
		f.basePath,
		err,
		f.basePath,
		tpath,
	)

	if f.core != nil {
		f.core.Close()
		f.core = nil
	}
	f.core, err = core.New(append(opts, core.WithIndexPath(tpath))...)
	if err != nil {
		return err
	}

	if f.kvs == nil {
		f.kvs = kvs.New(kvs.WithConcurrency(f.kvsdbConcurrency))
	} else if f.kvs.Len() > 0 {
		f.kvs.Close()
		f.kvs = kvs.New(kvs.WithConcurrency(f.kvsdbConcurrency))
	}

	return nil
}

func (f *faiss) load(ctx context.Context, path string, opts ...core.Option) error {
	exist, fi, err := file.ExistsWithDetail(path)
	switch {
	case !exist, fi == nil, fi != nil && fi.Size() == 0, err != nil && errors.Is(err, fs.ErrNotExist):
		// TODO errors/faiss.go
		err = errors.Wrapf(errors.ErrIndexFileNotFound, "index file does not exists,\tpath: %s,\terr: %v", path, err)
		return err
	case err != nil && errors.Is(err, fs.ErrPermission):
		if fi != nil {
			// TODO errors/faiss.go
			err = errors.Wrap(errors.ErrFailedToOpenFile(err, path, 0, fi.Mode()), "invalid permission for loading index path")
		}
		return err
	case exist && fi != nil && fi.IsDir():
		if fi.Mode().IsDir() && !strings.HasSuffix(path, string(os.PathSeparator)) {
			path += string(os.PathSeparator)
		}
		files, err := filepath.Glob(file.Join(filepath.Dir(path), "*"))
		if err != nil || len(files) == 0 {
			// TODO errors/faiss.go
			err = errors.Wrapf(errors.ErrIndexFileNotFound, "index path exists but no file does not exists in the directory,\tpath: %s,\tfiles: %v\terr: %v", path, files, err)
			return err
		}
		if strings.HasSuffix(path, string(os.PathSeparator)) {
			path = strings.TrimSuffix(path, string(os.PathSeparator))
		}
	}

	metadataPath := file.Join(path, metadata.AgentMetadataFileName)
	log.Debugf("index path: %s exists, now starting to check metadata from %s", path, metadataPath)
	exist, fi, err = file.ExistsWithDetail(metadataPath)
	switch {
	case !exist, fi == nil, fi != nil && fi.Size() == 0, err != nil && errors.Is(err, fs.ErrNotExist):
		// TODO errors/faiss.go
		err = errors.Wrapf(errors.ErrIndexFileNotFound, "metadata file does not exists,\tpath: %s,\terr: %v", metadataPath, err)
		return err
	case err != nil && errors.Is(err, fs.ErrPermission):
		if fi != nil {
			// TODO errors/faiss.go
			err = errors.Wrap(errors.ErrFailedToOpenFile(err, metadataPath, 0, fi.Mode()), "invalid permission for loading metadata")
		}
		return err
	}

	log.Debugf("index path: %s and metadata: %s exists, now starting to load metadata", path, metadataPath)
	agentMetadata, err := metadata.Load(metadataPath)
	if err != nil && errors.Is(err, fs.ErrNotExist) || agentMetadata == nil || agentMetadata.Faiss == nil || agentMetadata.Faiss.IndexCount == 0 {
		err = errors.Wrapf(err, "cannot read metadata from path: %s\tmetadata: %s", path, agentMetadata)
		return err
	}

	kvsFilePath := file.Join(path, kvsFileName)
	log.Debugf("index path: %s and metadata: %s exists and successfully load metadata, now starting to load kvs data from %s", path, metadataPath, kvsFilePath)
	exist, fi, err = file.ExistsWithDetail(kvsFilePath)
	switch {
	case !exist, fi == nil, fi != nil && fi.Size() == 0, err != nil && errors.Is(err, fs.ErrNotExist):
		// TODO errors/faiss.go
		err = errors.Wrapf(errors.ErrIndexFileNotFound, "kvsdb file does not exists,\tpath: %s,\terr: %v", kvsFilePath, err)
		return err
	case err != nil && errors.Is(err, fs.ErrPermission):
		if fi != nil {
			// TODO errors/faiss.go
			err = errors.ErrFailedToOpenFile(err, kvsFilePath, 0, fi.Mode())
		}
		err = errors.Wrapf(err, "invalid permission for loading kvsdb file from %s", kvsFilePath)
		return err
	}

	var timeout time.Duration
	if agentMetadata != nil && agentMetadata.Faiss != nil {
		log.Debugf("the backup index size is %d. starting to load...", agentMetadata.Faiss.IndexCount)
		timeout = time.Duration(
			math.Min(
				math.Max(
					float64(agentMetadata.Faiss.IndexCount)*float64(f.litFactor),
					float64(f.minLit),
				),
				float64(f.maxLit),
			),
		)
	} else {
		log.Debugf("cannot inspect the backup index size. starting to load default value.")
		timeout = time.Duration(math.Min(float64(f.minLit), float64(f.maxLit)))
	}

	log.Debugf("index path: %s and metadata: %s and kvsdb file: %s exists and successfully load metadata, now starting to load full index and kvs data in concurrent", path, metadataPath, kvsFilePath)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	eg, _ := errgroup.New(ctx)
	eg.Go(safety.RecoverFunc(func() (err error) {
		if f.core != nil {
			f.core.Close()
			f.core = nil
		}
		f.core, err = core.Load(append(opts, core.WithIndexPath(path))...)
		if err != nil {
			err = errors.Wrapf(err, "failed to load faiss index from path: %s", path)
			return err
		}
		return nil
	}))

	eg.Go(safety.RecoverFunc(func() (err error) {
		err = f.loadKVS(kvsFilePath)
		if err != nil {
			err = errors.Wrapf(err, "failed to load kvsdb data from path: %s", kvsFilePath)
			return err
		}
		if f.kvs != nil && float64(agentMetadata.Faiss.IndexCount/2) > float64(f.kvs.Len()) {
			// TODO errors/faiss.go
			return errors.ErrIndicesAreTooFewComparedToMetadata
		}
		return nil
	}))

	ech := make(chan error, 1)
	// NOTE: when it exceeds the timeout while loading,
	// it should exit this function and leave this goroutine running.
	f.eg.Go(safety.RecoverFunc(func() error {
		defer close(ech)
		ech <- safety.RecoverFunc(func() (err error) {
			err = eg.Wait()
			if err != nil {
				log.Error(err)
				return err
			}
			cancel()
			return nil
		})()
		return nil
	}))

	select {
	case err := <-ech:
		return err
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			log.Errorf("cannot load index backup data from %s within the timeout %s. the process is going to be killed.", path, timeout)
			err := metadata.Store(metadataPath,
				&metadata.Metadata{
					IsInvalid: true,
					Faiss: &metadata.Faiss{
						IndexCount: 0,
					},
				},
			)
			if err != nil {
				return err
			}
			// TODO errors/faiss.go
			return errors.ErrIndexLoadTimeout
		}
	}

	return nil
}

func (f *faiss) loadKVS(path string) error {
	gob.Register(map[string]uint32{})

	var (
		fi *os.File
		err error
	)

	fi, err = file.Open(
		path,
		os.O_RDONLY|os.O_SYNC,
		fs.ModePerm,
	)
	if err != nil {
		return err
	}
	defer func() {
		if fi != nil {
			derr := fi.Close()
			if derr != nil {
				err = errors.Wrap(err, derr.Error())
			}
		}
	}()

	m := make(map[string]uint32)
	err = gob.NewDecoder(fi).Decode(&m)
	if err != nil {
		log.Errorf("error decoding kvsdb file,\terr: %v", err)
		return err
	}

	if f.kvs == nil {
		f.kvs = kvs.New(kvs.WithConcurrency(f.kvsdbConcurrency))
	} else if f.kvs.Len() > 0 {
		f.kvs.Close()
		f.kvs = kvs.New(kvs.WithConcurrency(f.kvsdbConcurrency))
	}

	for k, id := range m {
		f.kvs.Set(k, id)
	}

	return nil
}

func (f *faiss) mktmp() error {
	if !f.enableCopyOnWrite {
		return nil
	}

	path, err := file.MkdirTemp(file.Join(os.TempDir(), "vald"))
	if err != nil {
		log.Warnf("failed to create temporary index file path directory %s:\terr: %v", path, err)
		return err
	}

	f.tmpPath.Store(path)

	return nil
}

func (f *faiss) Start(ctx context.Context) <-chan error {
	if f.dcd {
		return nil
	}

	ech := make(chan error, 2)
	f.eg.Go(safety.RecoverFunc(func() (err error) {
		defer close(ech)
		if f.dur <= 0 {
			f.dur = math.MaxInt64
		}
		if f.sdur <= 0 {
			f.sdur = math.MaxInt64
		}
		if f.lim <= 0 {
			f.lim = math.MaxInt64
		}

		if f.idelay > 0 {
			timer := time.NewTimer(f.idelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			timer.Stop()
		}

		tick := time.NewTicker(f.dur)
		sTick := time.NewTicker(f.sdur)
		limit := time.NewTicker(f.lim)
		defer tick.Stop()
		defer sTick.Stop()
		defer limit.Stop()
		for {
			err = nil
			select {
			case <-ctx.Done():
				err = f.CreateIndex(ctx)
				// TODO errors/faiss.go
				if err != nil && !errors.Is(err, errors.ErrUncommittedIndexNotFound) {
					ech <- err
					return errors.Wrap(ctx.Err(), err.Error())
				}
				return ctx.Err()
			case <-tick.C:
				if f.vq.IVQLen() >= f.alen {
					err = f.CreateIndex(ctx)
				}
			case <-limit.C:
				err = f.CreateAndSaveIndex(ctx)
			case <-sTick.C:
				err = f.SaveIndex(ctx)
			}
			if err != nil && err != errors.ErrUncommittedIndexNotFound {
				ech <- err
				runtime.Gosched()
				err = nil
			}
		}
	}))


	return ech
}

func (f *faiss) Train(nb int, xb []float32) error {
	err := f.core.Train(nb, xb)
	if err != nil {
		return err
	}

	return nil
}

func (f *faiss) Insert(uuid string, vec []float32) error {
	return f.insert(uuid, vec, time.Now().UnixNano(), true)
}

func (f *faiss) InsertWithTime(uuid string, vec []float32, t int64) error {
	if t <= 0 {
		t = time.Now().UnixNano()
	}

	return f.insert(uuid, vec, t, true)
}

func (f *faiss) insert(uuid string, xb []float32, t int64, validation bool) error {
	if len(uuid) == 0 {
		// TODO errors/faiss.go
		err := errors.ErrUUIDNotFound(0)
		return err
	}

	if validation {
		_, ok := f.Exists(uuid)
		if ok {
			// TODO errors/faiss.go
			return errors.ErrUUIDAlreadyExists(uuid)
		}
	}

	return f.vq.PushInsert(uuid, xb, t)
}

func (f *faiss) CreateIndex(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "vald/agent-faiss/service/Faiss.CreateIndex")
	defer func() {
		if span != nil {
			span.End()
		}
	}()

	ic := f.vq.IVQLen() + f.vq.DVQLen()
	if ic == 0 {
		// TODO errors/faiss.go
		return errors.ErrUncommittedIndexNotFound
	}

	err := func() error {
		ticker := time.NewTicker(time.Millisecond * 100)
		defer ticker.Stop()
		// wait for not indexing & not saving
		for f.IsIndexing() || f.IsSaving() {
			runtime.Gosched()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
		}
		return nil
	}()
	if err != nil {
		return err
	}

	f.cimu.Lock()
	defer f.cimu.Unlock()
	f.indexing.Store(true)
	defer f.indexing.Store(false)
	defer f.gc()
	now := time.Now().UnixNano()
	ic = f.vq.IVQLen() + f.vq.DVQLen()
	if ic == 0 {
		// TODO errors/faiss.go
		return errors.ErrUncommittedIndexNotFound
	}

	log.Infof("create index operation started, uncommitted indexes = %d", ic)
	log.Debug("create index delete phase started")
	f.vq.RangePopDelete(ctx, now, func(uuid string) bool {
		oid, ok := f.kvs.Delete(uuid)
		if !ok {
			// TODO errors/faiss.go
			log.Warn(errors.ErrObjectIDNotFound(uuid))
			return true
		}
		err := f.core.Remove(1, []int64{int64(oid)})
		if err != nil {
			log.Errorf("failed to remove oid: %d from faiss index. error: %v", oid, err)
			f.fmu.Lock()
			f.fmap[uuid] = oid
			f.fmu.Unlock()
		}
		return true
	})
	log.Debug("create index delete phase finished")

	f.gc()

	log.Debug("create index insert phase started")
	f.vq.RangePopInsert(ctx, now, func(uuid string, vector []float32) bool {
		err := f.Train(1, vector)
		if err != nil {
			log.Errorf("failed to train vector uuid: %s vec: %v to faiss index. error: %v", uuid, vector, err)
			return true
		}
		err = f.core.Add(1, vector, []int64{int64(f.icnt)})
		if err != nil {
			log.Errorf("failed to insert vector uuid: %s vec: %v to faiss index. error: %v", uuid, vector, err)
			return true
		} else {
			f.kvs.Set(uuid, uint32(f.icnt))
			atomic.AddUint64(&f.icnt, 1)
		}
		f.fmu.Lock()
		_, ok := f.fmap[uuid]
		if ok {
			delete(f.fmap, uuid)
		}
		f.fmu.Unlock()
		return true
	})
	log.Debug("create index insert phase finished")

	// TODO re-Train()?

	atomic.AddUint64(&f.nocie, 1)

	log.Info("create index operation finished")

	return nil
}

func (f *faiss) SaveIndex(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "vald/agent-faiss/service/Faiss.SaveIndex")
	defer func() {
		if span != nil {
			span.End()
		}
	}()

	if !f.inMem {
		return f.saveIndex(ctx)
	}

	return nil
}

func (f *faiss) saveIndex(ctx context.Context) error {
	nocie := atomic.LoadUint64(&f.nocie)
	if atomic.LoadUint64(&f.lastNocie) == nocie {
		return nil
	}
	atomic.SwapUint64(&f.lastNocie, nocie)

	err := func() error {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		// wait for not indexing & not saving
		for f.IsIndexing() || f.IsSaving() {
			runtime.Gosched()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
		}
		return nil
	}()
	if err != nil {
		return err
	}

	f.saving.Store(true)
	defer f.gc()
	defer f.saving.Store(false)

	// no cleanup invalid index

	eg, ectx := errgroup.New(ctx)
	// we want to ensure the acutal kvs size between kvsdb and metadata,
	// so we create this counter to count the actual kvs size instead of using kvs.Len()
	var (
		kvsLen uint64
		path   string
	)

	if f.enableCopyOnWrite {
		path = f.tmpPath.Load().(string)
	} else {
		path = f.path
	}

	f.smu.Lock()
	defer f.smu.Unlock()

	eg.Go(safety.RecoverFunc(func() (err error) {
		if f.kvs.Len() > 0 && path != "" {
			m := make(map[string]uint32, f.Len())
			var mu sync.Mutex

			f.kvs.Range(ectx, func(key string, id uint32) bool {
				mu.Lock()
				m[key] = id
				mu.Unlock()
				atomic.AddUint64(&kvsLen, 1)
				return true
			})

			var fi *os.File
			fi, err = file.Open(
				file.Join(path, kvsFileName),
				os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
				fs.ModePerm,
			)
			if err != nil {
				return err
			}
			defer func() {
				if fi != nil {
					derr := fi.Close()
					if derr != nil {
						err = errors.Wrap(err, derr.Error())
					}
				}
			}()

			gob.Register(map[string]uint32{})
			err = gob.NewEncoder(fi).Encode(&m)
			if err != nil {
				return err
			}

			err = fi.Sync()
			if err != nil {
				return err
			}

			m = make(map[string]uint32)
		}

		return nil
	}))

	eg.Go(safety.RecoverFunc(func() (err error) {
		f.fmu.Lock()
		fl := len(f.fmap)
		f.fmu.Unlock()

		if fl > 0 && path != "" {
			var fi *os.File
			fi, err = file.Open(
				file.Join(path, "invalid-"+kvsFileName),
				os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
				fs.ModePerm,
			)
			if err != nil {
				return err
			}
			defer func() {
				if fi != nil {
					derr := fi.Close()
					if derr != nil {
						err = errors.Wrap(err, derr.Error())
					}
				}
			}()

			gob.Register(map[string]uint32{})
			f.fmu.Lock()
			err = gob.NewEncoder(fi).Encode(&f.fmap)
			f.fmu.Unlock()
			if err != nil {
				return err
			}

			err = fi.Sync()
			if err != nil {
				return err
			}
		}

		return nil
	}))

	eg.Go(safety.RecoverFunc(func() error {
		return f.core.SaveIndexWithPath(path)
	}))

	err = eg.Wait()
	if err != nil {
		return err
	}

	err = metadata.Store(
		file.Join(path, metadata.AgentMetadataFileName),
		&metadata.Metadata{
			IsInvalid: false,
			Faiss: &metadata.Faiss{
				IndexCount: kvsLen,
			},
		},
	)
	if err != nil {
		return err
	}

	return f.moveAndSwitchSavedData(ctx)
}

func (f *faiss) moveAndSwitchSavedData(ctx context.Context) error {
	if !f.enableCopyOnWrite {
		return nil
	}

	var err error
	f.cowmu.Lock()
	defer f.cowmu.Unlock()

	err = file.MoveDir(ctx, f.path, f.oldPath)
	if err != nil {
		log.Warnf("failed to backup backup data from %s to %s error: %v", f.path, f.oldPath, err)
	}

	path := f.tmpPath.Load().(string)
	err = file.MoveDir(ctx, path, f.path)
	if err != nil {
		log.Warnf("failed to move temporary index data from %s to %s error: %v, trying to rollback secondary backup data from %s to %s", path, f.path, f.oldPath, f.path, err)
		return file.MoveDir(ctx, f.oldPath, f.path)
	}
	defer log.Warnf("finished to copy index from %s => %s => %s", path, f.path, f.oldPath)

	return f.mktmp()
}

func (f *faiss) CreateAndSaveIndex(ctx context.Context) error {
	ctx, span := trace.StartSpan(ctx, "vald/agent-faiss/service/Faiss.CreateAndSaveIndex")
	defer func() {
		if span != nil {
			span.End()
		}
	}()

	err := f.CreateIndex(ctx)
	// TODO errors/faiss.go
	if err != nil &&
		!errors.Is(err, errors.ErrUncommittedIndexNotFound) &&
		!errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	return f.SaveIndex(ctx)
}

func (f *faiss) Search(k, nq uint32, xq []float32) ([]model.Distance, error) {
	if f.IsIndexing() {
		// TODO errors/faiss.go
		return nil, errors.ErrCreateIndexingIsInProgress
	}

	sr, err := f.core.Search(int(k), int(nq), xq)
	if err != nil {
		if f.IsIndexing() {
			// TODO errors/faiss.go
			return nil, errors.ErrCreateIndexingIsInProgress
		}

		log.Errorf("cgo error detected: faiss api returned error %v", err)
		return nil, err
	}

	if len(sr) == 0 {
		// TODO errors/faiss.go
		return nil, errors.ErrEmptySearchResult
	}

	ds := make([]model.Distance, 0, len(sr))
	for _, d := range sr {
		if err = d.Error; d.ID == 0 && err != nil {
			log.Warnf("an error occurred while searching: %s", err)
			continue
		}

		key, ok := f.kvs.GetInverse(d.ID)
		if ok {
			ds = append(ds, model.Distance{
				ID:       key,
				Distance: d.Distance,
			})
		} else {
			log.Warn("not found", d.ID, d.Distance)
		}
	}

	return ds, nil
}

func (f *faiss) Delete(uuid string) (err error) {
	return f.delete(uuid, time.Now().UnixNano(), true)
}

func (f *faiss) DeleteWithTime(uuid string, t int64) (err error) {
	if t <= 0 {
		t = time.Now().UnixNano()
	}

	return f.delete(uuid, t, true)
}

func (f *faiss) delete(uuid string, t int64, validation bool) error {
	if len(uuid) == 0 {
		// TODO errors/faiss.go
		return errors.ErrUUIDNotFound(0)
	}

	if validation {
		_, ok := f.kvs.Get(uuid)
		if !ok && !f.vq.IVExists(uuid) {
			// TODO errors/faiss.go
			return errors.ErrObjectIDNotFound(uuid)
		}
	}

	return f.vq.PushDelete(uuid, t)
}

func (f *faiss) Exists(uuid string) (uint32, bool) {
	var (
		oid uint32
		ok  bool
	)

	ok = f.vq.IVExists(uuid)
	if !ok {
		oid, ok = f.kvs.Get(uuid)
		if !ok {
			// TODO errors/faiss.go
			log.Debugf("Exists\tuuid: %s's data not found in kvsdb and insert vqueue\terror: %v", uuid, errors.ErrObjectIDNotFound(uuid))
			return 0, false
		}
		if f.vq.DVExists(uuid) {
			// TODO errors/faiss.go
			log.Debugf("Exists\tuuid: %s's data found in kvsdb and not found in insert vqueue, but delete vqueue data exists. the object will be delete soon\terror: %v",
				uuid, errors.ErrObjectIDNotFound(uuid))
			return 0, false
		}
	}

	return oid, ok
}

func (f *faiss) IsIndexing() bool {
	i, ok := f.indexing.Load().(bool)
	return i && ok
}

func (f *faiss) IsSaving() bool {
	s, ok := f.saving.Load().(bool)
	return s && ok
}

func (f *faiss) NumberOfProactiveGCExecution() uint64 {
	return atomic.LoadUint64(&f.nogce)
}

func (f *faiss) gc() {
	if f.enableProactiveGC {
		runtime.GC()
		atomic.AddUint64(&f.nogce, 1)
	}
}

func (f *faiss) Len() uint64 {
	return f.kvs.Len()
}

func (f *faiss) GetDimensionSize() int {
	return f.dim
}

func (f *faiss) Close(ctx context.Context) error {
	err := f.kvs.Close()
	if len(f.path) != 0 {
		cerr := f.CreateIndex(ctx)
		// TODO errors/faiss.go
		if cerr != nil &&
			!errors.Is(err, errors.ErrUncommittedIndexNotFound) &&
			!errors.Is(err, context.Canceled) &&
			!errors.Is(err, context.DeadlineExceeded) {
			if err != nil {
				err = errors.Wrap(cerr, err.Error())
			} else {
				err = cerr
			}
		}

		serr := f.SaveIndex(ctx)
		// TODO errors/faiss.go
		if serr != nil &&
			!errors.Is(err, errors.ErrUncommittedIndexNotFound) &&
			!errors.Is(err, context.Canceled) &&
			!errors.Is(err, context.DeadlineExceeded) {
			if err != nil {
				err = errors.Wrap(serr, err.Error())
			} else {
				err = serr
			}
		}
	}

	f.core.Close()

	return nil
}
