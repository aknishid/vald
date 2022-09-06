package grpc

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vdaas/vald/apis/grpc/v1/payload"
	"github.com/vdaas/vald/internal/config"
	"github.com/vdaas/vald/internal/core/algorithm/ngt"
	"github.com/vdaas/vald/internal/errgroup"
	"github.com/vdaas/vald/internal/net/grpc/codes"
	"github.com/vdaas/vald/internal/net/grpc/status"
	"github.com/vdaas/vald/pkg/agent/core/ngt/service"
)

func Test_server_Insert_Update_parallel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defaultF32SvcCfg := &config.NGT{
		Dimension:    3,
		DistanceType: ngt.Angle.String(),
		ObjectType:   ngt.Float.String(),
		KVSDB:        &config.KVSDB{},
		VQueue:       &config.VQueue{},
	}

	eg, _ := errgroup.New(ctx)
	ngt, err := service.New(defaultF32SvcCfg,
		service.WithErrGroup(eg),
	)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(WithNGT(ngt), WithErrGroup(eg))
	if err != nil {
		t.Errorf("failed to init service, err: %v", err)
	}

	insertReq := &payload.Insert_Request{
		Vector: &payload.Object_Vector{
			Id:     "sameid",
			Vector: []float32{1, 2, 3},
		},
		Config: &payload.Insert_Config{
			// default is do strict exist check, uncomment below line to skip strict exist check
			// SkipStrictExistCheck: true,
			// Timestamp: 10, // insert timestamp,
		},
	}
	updateReq := &payload.Update_Request{
		Vector: &payload.Object_Vector{
			Id:     "sameid",
			Vector: []float32{3, 2, 1},
		},
		Config: &payload.Update_Config{
			// default is do strict exist check, uncomment below line to skip strict exist check
			// SkipStrictExistCheck: true,
			// Timestamp: 10, // insert timestamp,
		},
	}

	// perform insert
	insertCnt := 20
	start := make(chan struct{}) // control the execution of insert goroutines
	wg := &sync.WaitGroup{}
	wg.Add(insertCnt)

	var successInsertCnt int32 = 0
	for i := 0; i < insertCnt; i++ {
		go func() {
			<-start // wait for start channel is closed to start this goroutine at the same time (as close as it can)
			defer wg.Done()

			_, err := s.Insert(ctx, insertReq) // insert the same insert request
			if err == nil {
				atomic.AddInt32(&successInsertCnt, 1) // add successs count 1 if no error is returned from Insert()
				return
			}

			// convert the error and ensure it is already exists error
			st, ok := status.FromError(err)
			if !ok {
				t.Errorf("error cannot convert to status, %v", err)
			}
			if st.Code() != codes.AlreadyExists {
				t.Errorf("unexpected status code, %v", st)
			}
		}()
	}

	// perform update
	updateCnt := 20
	wg.Add(updateCnt)

	var successUpdateCnt int32 = 0
	for i := 0; i < updateCnt; i++ {
		go func() {
			<-start // wait for start channel is closed to start this goroutine at the same time (as close as it can)
			defer wg.Done()

			_, err := s.Update(ctx, updateReq) // update the same insert request
			if err == nil {
				atomic.AddInt32(&successUpdateCnt, 1) // add successs count 1 if no error is returned
			}
		}()
	}

	close(start) // start the insert goroutines at the same time (as close as it can)
	wg.Wait()

	if _, err = s.CreateIndex(ctx, &payload.Control_CreateIndexRequest{
		PoolSize: 100,
	}); err != nil {
		t.Error(err)
	}

	v, err := s.GetObject(ctx, &payload.Object_VectorRequest{
		Id: &payload.Object_ID{
			Id: "sameid",
		},
	})
	if err != nil {
		t.Error(err)
	}
	if v == nil || v.GetId() != "sameid" {
		t.Errorf("unexpected object, got: %#v", v)
	}

	res, err := s.Search(ctx, &payload.Search_Request{
		Vector: v.GetVector(),
		Config: &payload.Search_Config{
			Num:    100,
			Radius: 1.0,
		},
	})
	if err != nil {
		t.Error(err)
	}

	if len(res.GetResults()) != 1 {
		t.Errorf("expected result count 1, req: %#v, got: %#v", v, res)
	}
}
