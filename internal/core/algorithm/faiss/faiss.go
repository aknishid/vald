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

// Package faiss provides implementation of Go API for https://github.com/facebookresearch/faiss
package faiss

/*
#cgo LDFLAGS: -lfaiss
#include <Capi.h>
*/
import "C"

import (
	"unsafe"

	"github.com/vdaas/vald/internal/errors"
)

type (
	// Faiss is core interface.
	Faiss interface {
		// SaveIndex stores faiss index to strage.
		SaveIndex() error

		// SaveIndexWithPath stores faiss index to specified storage.
		SaveIndexWithPath(idxPath string) error

		// TODO comment
		Train(nb int, xb []float32) error

		// TODO comment
		Add(nb int, xb []float32, xids []int64) error

		// Search returns search result as []SearchResult.
		Search(k, nq int, xq []float32) ([]SearchResult, error)

		// Remove removes from faiss index.
		Remove(size int, ids []int64) error

		// Close faiss index.
		Close()
	}

	faiss struct {
		st          *C.FaissStruct
		dimension   C.int
		nlist       C.int
		m           C.int
		nbitsPerIdx C.int
		idxPath     string
	}

	SearchResult struct {
		ID       uint32
		Distance float32
		Error    error
	}
)

// New returns Faiss instance with recreating empty index file.
func New(opts ...Option) (Faiss, error) {
	return gen(false, opts...)
}

func Load(opts ...Option) (Faiss, error) {
	return gen(true, opts...)
}

func gen(isLoad bool, opts ...Option) (Faiss, error) {
	var (
		f   = new(faiss)
		err error
	)

	for _, opt := range append(defaultOptions, opts...) {
		if err = opt(f); err != nil {
			return nil, errors.New("faiss option error")
		}
	}

	if isLoad {
		path := C.CString(f.idxPath)
		defer C.free(unsafe.Pointer(path))
		f.st = C.faiss_read_index(path)
		if f.st == nil {
			return nil, errors.New("faiss create index error")
		}
	} else {
		f.st = C.faiss_create_index(f.dimension, f.nlist, f.m, f.nbitsPerIdx)
		if f.st == nil {
			return nil, errors.New("faiss create index error")
		}
	}

	return f, nil
}

// SaveIndex stores faiss index to storage.
func (f *faiss) SaveIndex() error {
	path := C.CString(f.idxPath)
	defer C.free(unsafe.Pointer(path))
	C.faiss_write_index(f.st, path)

	return nil
}

// SaveIndexWithPath stores faiss index to specified storage.
func (f *faiss) SaveIndexWithPath(idxPath string) error {
	path := C.CString(idxPath)
	defer C.free(unsafe.Pointer(path))
	C.faiss_write_index(f.st, path)

	return nil
}

// TODO comment
func (f *faiss) Train(nb int, xb []float32) error {
	C.faiss_train(f.st, (C.int)(nb), (*C.float)(&xb[0]))

	return nil
}

// TODO comment
func (f *faiss) Add(nb int, xb []float32, xids []int64) error {
	C.faiss_add(f.st, (C.int)(nb), (*C.float)(&xb[0]), (*C.long)(&xids[0]))

	return nil
}

// Search returns search result as []SearchResult.
func (f *faiss) Search(k, nq int, xq []float32) ([]SearchResult, error) {
	I := make([]int64, k*nq)
	D := make([]float32, k*nq)

	C.faiss_search(f.st, (C.int)(k), (C.int)(nq), (*C.float)(&xq[0]), (*C.long)(&I[0]), (*C.float)(&D[0]))

	result := make([]SearchResult, k)
	for i := range result {
		result[i] = SearchResult{uint32(I[i]), D[i], nil}
	}

	return result, nil
}

// Remove removes from faiss index.
func (f *faiss) Remove(size int, ids []int64) error {
	C.faiss_remove(f.st, (C.int)(size), (*C.long)(&ids[0]))

	return nil
}

// Close faiss index.
func (f *faiss) Close() {
	if f.st != nil {
		C.faiss_free(f.st)
		f.st = nil
	}
}
