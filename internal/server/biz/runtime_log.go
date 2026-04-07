package biz

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/samber/lo"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/runtimelog"
	"github.com/looplj/axonhub/internal/log"
)

type RuntimeLogService struct {
	*AbstractService

	queue chan log.RuntimeLogRecord
	wg    sync.WaitGroup
}

func NewRuntimeLogService(ent *ent.Client) *RuntimeLogService {
	return &RuntimeLogService{
		AbstractService: &AbstractService{db: ent},
		queue:           make(chan log.RuntimeLogRecord, 2048),
	}
}

func (s *RuntimeLogService) Start(_ context.Context) error {
	s.wg.Add(1)

	go func() {
		defer s.wg.Done()

		for record := range s.queue {
			if err := s.persist(record); err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "runtime log persistence failed: %v\n", err)
			}
		}
	}()

	return nil
}

func (s *RuntimeLogService) Stop(_ context.Context) error {
	close(s.queue)
	s.wg.Wait()
	return nil
}

func (s *RuntimeLogService) WriteRuntimeLog(_ context.Context, record log.RuntimeLogRecord) {
	s.queue <- record
}

func (s *RuntimeLogService) persist(record log.RuntimeLogRecord) error {
	ctx := log.WithoutRuntimeLogSink(authz.WithSystemBypass(context.Background(), "runtime-log-persist"))
	client := s.entFromContext(ctx)

	mut := client.RuntimeLog.Create().
		SetCreatedAt(record.CreatedAt).
		SetLogger(record.Logger).
		SetLevel(runtimelog.Level(record.Level)).
		SetMessage(record.Message).
		SetCaller(record.Caller).
		SetFieldsJSON(lo.Ternary(record.FieldsJSON != nil, record.FieldsJSON, map[string]any{}))

	if record.TraceID != "" {
		mut = mut.SetTraceID(record.TraceID)
	}
	if record.RequestID != "" {
		mut = mut.SetRequestID(record.RequestID)
	}
	if record.OperationName != "" {
		mut = mut.SetOperationName(record.OperationName)
	}
	if record.ChannelID != nil {
		mut = mut.SetChannelID(*record.ChannelID)
	}
	if record.ChannelName != "" {
		mut = mut.SetChannelName(record.ChannelName)
	}
	if record.ModelID != "" {
		mut = mut.SetModelID(record.ModelID)
	}

	_, err := mut.Save(ctx)
	return err
}
