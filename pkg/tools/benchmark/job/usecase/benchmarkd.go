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

// Package usecase provides usecases
package usecase

import (
	"context"

	"github.com/vdaas/vald/internal/client/v1/client/vald"
	iconf "github.com/vdaas/vald/internal/config"
	"github.com/vdaas/vald/internal/errgroup"
	"github.com/vdaas/vald/internal/errors"
	"github.com/vdaas/vald/internal/log"
	"github.com/vdaas/vald/internal/net/grpc"
	"github.com/vdaas/vald/internal/net/grpc/interceptor/server/recover"
	"github.com/vdaas/vald/internal/observability"
	infometrics "github.com/vdaas/vald/internal/observability/metrics/info"
	"github.com/vdaas/vald/internal/runner"
	"github.com/vdaas/vald/internal/safety"
	"github.com/vdaas/vald/internal/servers/server"
	"github.com/vdaas/vald/internal/servers/starter"
	"github.com/vdaas/vald/internal/test/data/hdf5"
	"github.com/vdaas/vald/pkg/tools/benchmark/job/config"
	handler "github.com/vdaas/vald/pkg/tools/benchmark/job/handler/grpc"
	"github.com/vdaas/vald/pkg/tools/benchmark/job/handler/rest"
	"github.com/vdaas/vald/pkg/tools/benchmark/job/router"
	"github.com/vdaas/vald/pkg/tools/benchmark/job/service"
)

type run struct {
	eg            errgroup.Group
	cfg           *config.Config
	job           service.Job
	h             handler.Benchmark
	server        starter.Server
	observability observability.Observability
}

func New(cfg *config.Config) (r runner.Runner, err error) {
	log.Info("pkg/tools/benchmark/job/cmd start")
	eg := errgroup.Get()
	copts, err := cfg.Job.GatewayClient.Opts()
	if err != nil {
		return nil, err
	}

	c, err := vald.New(
		vald.WithAddrs(cfg.Job.GatewayClient.Addrs...),
		vald.WithClient(grpc.New(copts...)),
	)
	if err != nil {
		return nil, err
	}

	d, err := hdf5.New(
		hdf5.WithNameByString(cfg.Job.Dataset.Name),
	)
	if err != nil {
		return nil, err
	}
	log.Info("pkg/tools/benchmark/job/cmd success d")

	job, err := service.New(
		service.WithErrGroup(eg),
		service.WithValdClient(c),
		service.WithJobTypeByString(cfg.Job.JobType),
		service.WithDimension(cfg.Job.Dimension),
		service.WithIter(cfg.Job.Iter),
		service.WithNum(cfg.Job.Num),
		service.WithMinNum(cfg.Job.MinNum),
		service.WithRadius(cfg.Job.Radius),
		service.WithEpsilon(cfg.Job.Epsilon),
		service.WithTimeout(cfg.Job.Timeout),
		service.WithHdf5(d),
	)
	if err != nil {
		return nil, err
	}

	h, err := handler.New()
	if err != nil {
		return nil, err
	}

	grpcServerOptions := []server.Option{
		server.WithGRPCRegistFunc(func(srv *grpc.Server) {
			// TODO register grpc server handler here
		}),
		server.WithGRPCOption(
			grpc.ChainUnaryInterceptor(recover.RecoverInterceptor()),
			grpc.ChainStreamInterceptor(recover.RecoverStreamInterceptor()),
		),
		server.WithPreStartFunc(func() error {
			// TODO check unbackupped upstream
			return nil
		}),
		server.WithPreStopFunction(func() error {
			// TODO backup all index data here
			return nil
		}),
	}

	var obs observability.Observability
	if cfg.Observability.Enabled {
		obs, err = observability.NewWithConfig(
			cfg.Observability,
			infometrics.New("vald_benchmark_job_info", "Benchmark Job info", *cfg.Job),
		)
		if err != nil {
			return nil, err
		}
	}

	srv, err := starter.New(
		starter.WithConfig(cfg.Server),
		starter.WithREST(func(sc *iconf.Server) []server.Option {
			return []server.Option{
				server.WithHTTPHandler(
					router.New(
						router.WithTimeout(sc.HTTP.HandlerTimeout),
						router.WithErrGroup(eg),
						router.WithHandler(
							rest.New(
							// TODO pass grpc handler to REST option
							),
						),
					)),
			}
		}),
		starter.WithGRPC(func(sc *iconf.Server) []server.Option {
			return grpcServerOptions
		}),
	)
	if err != nil {
		return nil, err
	}
	log.Info("pkg/tools/benchmark/job/cmd end")

	return &run{
		eg:            eg,
		cfg:           cfg,
		job:           job,
		h:             h,
		server:        srv,
		observability: obs,
	}, nil
}

func (r *run) PreStart(ctx context.Context) error {
	if r.observability != nil {
		if err := r.observability.PreStart(ctx); err != nil {
			return err
		}
	}
	if r.job != nil {
		return r.job.PreStart(ctx)
	}
	return nil
}

func (r *run) Start(ctx context.Context) (<-chan error, error) {
	ech := make(chan error, 3)
	var oech, dech, sech <-chan error
	r.eg.Go(safety.RecoverFunc(func() (err error) {
		defer close(ech)
		if r.observability != nil {
			oech = r.observability.Start(ctx)
		}
		dech, err = r.job.Start(ctx)

		if err != nil {
			ech <- err
			return err
		}

		r.h.Start(ctx)

		sech = r.server.ListenAndServe(ctx)

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case err = <-oech:
			case err = <-dech:
			case err = <-sech:
			}
			if err != nil {
				select {
				case <-ctx.Done():
					log.Error(err)
					return errors.Wrap(ctx.Err(), err.Error())
				case ech <- err:
				}
			}
		}
	}))
	return ech, nil
}

func (r *run) PreStop(ctx context.Context) error {
	return nil
}

func (r *run) Stop(ctx context.Context) error {
	if r.observability != nil {
		r.observability.Stop(ctx)
	}
	return r.server.Shutdown(ctx)
}

func (r *run) PostStop(ctx context.Context) error {
	return nil
}
