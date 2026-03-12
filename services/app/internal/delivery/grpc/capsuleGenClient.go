package grpc

import (
	"capsule-me/internal/domain/catalog"
	capsulegen "capsule-me/internal/gen/capsule-gen"
	"context"
	"errors"
	"io"
	"sync/atomic"

	"google.golang.org/grpc"
)

// это то, откуда воркеры берут работу, id надо для индентификации запроса, инкремент
type job struct {
	id  int32
	req *catalog.IncomingFeature
	ctx context.Context
}

type result struct {
	id   int32
	resp *catalog.Capsule
	err  error
}

type grpcCatalogClient struct {
	workers   int
	increment func() int32
	client    capsulegen.CapsuleGenServiceClient
	jobs      chan job
	results   chan result
}

func inc() func() int32 {
	var i atomic.Int32
	return func() int32 {
		i.Add(1)
		return i.Load()
	}
}

// доделать
func NewGrpcCatalogClient(maxWorkers int, baseURL string) *grpcCatalogClient {
	conn, err := grpc.NewClient(baseURL) // DialOptions сделать потом, они идут как второй аргумент ...DialOptions
	if err != nil {
		return nil
	}
	jobs := make(chan job, maxWorkers)
	results := make(chan result, maxWorkers)

	client := capsulegen.NewCapsuleGenServiceClient(conn)
	if client == nil {
		return nil
	}

	increment := inc()

	return &grpcCatalogClient{
		client:    client,
		jobs:      jobs,
		results:   results,
		workers:   maxWorkers,
		increment: increment,
	}
}

// заводит воркеров
func (cc *grpcCatalogClient) Start() {
	for i := 0; i < cc.workers; i++ {
		go func() {
			for j := range cc.jobs {
				select {
				case <-j.ctx.Done():
					continue // делаем так, чтобы воркер не умер, если надо одно задание пропустить
				default:
					res, err := cc.recommend(j.ctx, j.req)
					cc.results <- result{id: j.id, resp: res, err: err}
				}
			}
		}()
	}
}

// реализовать
func toDTO(in *catalog.IncomingFeature) (*capsulegen.CapsuleGenerateRequest, error)
func fromDTO(out *capsulegen.Capsule) (*catalog.Capsule, error)

func (cc *grpcCatalogClient) submit(job job) bool {
	select {
	case cc.jobs <- job:
		// надо как-то сделать так, что бы не записать в закрытый канал
		return true
	default:
		return false
	}
}
func (cc *grpcCatalogClient) recommend(ctx context.Context, f *catalog.IncomingFeature) (*catalog.Capsule, error) {
	dto, err := toDTO(f)
	if err != nil {
		return nil, err
	}

	stream, err := cc.client.CapsuleGenerate(ctx, dto)
	if err != nil {
		return nil, err
	}

	var capsule *capsulegen.Capsule
	var imageData []byte

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch v := resp.Payload.(type) {
		case *capsulegen.CapsuleGenerateResponse_Capsule:
			capsule = v.Capsule
			// тут можно логгировать

		case *capsulegen.CapsuleGenerateResponse_ImageChunk:
			imageData = append(imageData, v.ImageChunk...)
			// тут тоже логгировать

		default:
			// логгировать неизвестный тип payload
		}
	}

	if capsule == nil || len(imageData) == 0 {
		return nil, errors.New("nil капсула или картинка")
	}

	res, err := fromDTO(capsule)
	if err != nil {
		return nil, err
	}
	res.Image = imageData
	return res, nil
}

func (cc *grpcCatalogClient) Recommend(ctx context.Context, f *catalog.IncomingFeature) (*catalog.Capsule, error) {
	job := job{
		id:  cc.increment(),
		req: f,
		ctx: ctx,
	}

	if ok := cc.submit(job); !ok {
		return nil, errors.New("job не положилась в канал")
	}

	// всетаки оно читает все и выбрасывает неподходящие, надо иначе организовать
	for result := range cc.results {
		if result.id == job.id {
			return result.resp, result.err
		}
	}

	return nil, errors.New("not found")
}
