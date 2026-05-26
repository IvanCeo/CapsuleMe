package grpc

import (
	"capsule-me/internal/domain/catalog"
	capsulegen "capsule-me/internal/gen/capsule-gen"
	"capsule-me/internal/gen/common"
	lookgen "capsule-me/internal/gen/look-gen"
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type jobType int

const (
	getCapsule jobType = iota
	getLooks
)

// это то, откуда воркеры берут работу, id надо для индентификации запроса, инкремент
type job struct {
	id         int32
	capReq     *catalog.IncomingFeature
	lookReq    *catalog.Capsule
	ctx        context.Context
	capResult  chan capsuleResult
	lookResult chan lookResult
	jobtype    jobType
}

type capsuleResult struct {
	id   int32
	resp *catalog.Capsule
	err  error
}

type GrpcCatalogClient struct {
	mu            *sync.RWMutex
	wg            *sync.WaitGroup
	stopped       atomic.Bool
	workers       int
	increment     func() int32
	capsuleClient capsulegen.CapsuleGenServiceClient
	lookClient    lookgen.LookGenServiceClient
	jobs          chan job
	log           *slog.Logger
}

func inc() func() int32 {
	var i atomic.Int32
	return func() int32 {
		i.Add(1)
		return i.Load()
	}
}

func NewGrpcCatalogClient(maxWorkers int, logger *slog.Logger, capsuleURL, lookURL string) (*GrpcCatalogClient, error) {
	capsuleConn, err := grpc.NewClient(capsuleURL, grpc.WithTransportCredentials(insecure.NewCredentials())) // DialOptions сделать потом, они идут как второй аргумент ...DialOptions
	if err != nil {
		return nil, err
	}
	jobs := make(chan job, maxWorkers)

	capsuleClient := capsulegen.NewCapsuleGenServiceClient(capsuleConn)
	if capsuleClient == nil {
		return nil, errors.New("nil client in capsuleClient")
	}

	lookConn, err := grpc.NewClient(lookURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	lookClient := lookgen.NewLookGenServiceClient(lookConn)
	if lookClient == nil {
		return nil, errors.New("nil client in lookClient")
	}

	wg := &sync.WaitGroup{}
	mu := &sync.RWMutex{}
	increment := inc()

	return &GrpcCatalogClient{
		mu:            mu,
		wg:            wg,
		capsuleClient: capsuleClient,
		lookClient:    lookClient,
		jobs:          jobs,
		workers:       maxWorkers,
		increment:     increment,
		log:           logger,
	}, nil
}

// заводит воркеров
func (cc *GrpcCatalogClient) Start() {
	for i := 0; i < cc.workers; i++ {
		cc.wg.Add(1)
		go func() {
			defer cc.wg.Done()
			for j := range cc.jobs {
				switch j.jobtype {
				case getCapsule:
					res, err := cc.ForvardRecommend(j.ctx, j.capReq)
					j.capResult <- capsuleResult{id: j.id, resp: res, err: err}
				case getLooks:
					res, err := cc.ForvardRecommendLooks(j.ctx, j.lookReq)
					j.lookResult <- lookResult{id: j.id, resp: res, err: err}
				}
			}
		}()
	}
	cc.log.Info("grpc pool started")
}

func (cc *GrpcCatalogClient) Stop() {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.stopped.Store(true)
	close(cc.jobs)
	cc.log.Info("каналы закрыты")

	done := make(chan struct{})
	go func() {
		cc.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		cc.log.Info("воркеры остановлены")
	case <-time.After(5 * time.Second):
		cc.log.Error("таймаут ожидания воркеров")
	}
}

func toDTO(in *catalog.IncomingFeature) (*capsulegen.CapsuleGenerateRequest, error) {
	if in != nil {
		res := &capsulegen.CapsuleGenerateRequest{}

		switch in.Gender {
		case "male":
			res.Gender = common.Gender_GENDER_MALE
		case "female":
			res.Gender = common.Gender_GENDER_FEMALE
		default:
			res.Gender = common.Gender_GENDER_UNSPECIFIED
		}

		switch in.Style {
		case "classic":
			res.Style = common.Style_STYLE_CLASSIC
		case "casual":
			res.Style = common.Style_STYLE_CASUAL
		case "sport":
			res.Style = common.Style_STYLE_SPORT
		default:
			res.Style = common.Style_STYLE_UNSPECIFIED
		}

		switch in.Season {
		case "winter":
			res.Season = common.Season_SEASON_WINTER
		case "summer":
			res.Season = common.Season_SEASON_SUMMER
		case "autumn":
			res.Season = common.Season_SEASON_AUTUMN
		case "spring":
			res.Season = common.Season_SEASON_SPRING
		default:
			res.Season = common.Season_SEASON_UNSPECIFIED
		}

		switch in.Color {
		case "dark":
			res.Palette = common.Palette_PALETTE_DARK.Enum()
		case "light":
			res.Palette = common.Palette_PALETTE_LIGHT.Enum()
		case "bright":
			res.Palette = common.Palette_PALETTE_BRIGHT.Enum()
		case "none":
			res.Palette = nil
		default:
			res.Palette = common.Palette_PALETTE_UNSPECIFIED.Enum()
		}

		return res, nil
	}

	return nil, errors.New("grpc toDTO: in is nil")
}

// картинка перекладывается отдельно
func fromDTO(out *capsulegen.Capsule) (*catalog.Capsule, error) {
	if out != nil {
		res := &catalog.Capsule{}

		for _, item := range out.Item {
			i := &catalog.ImageItem{}
			// id, err := uuid.Parse(item.GetId())
			// if err != nil {
			// 	return nil, errors.New("invalid item ID")
			// }
			id := item.GetId()
			i.ID = id
			i.ObjectID = id
			i.Gender = item.GetGender().String()
			i.CategoryGroup = item.GetCategoryGroup()
			i.Category = item.GetCategory()
			i.Style = item.GetStyle().String()
			i.Color = item.GetColor()
			i.Season = item.GetSeason().String()
			i.Material = item.GetMaterial()
			i.Description = item.GetDescription()
			i.Ext = item.GetExt()

			res.Items = append(res.Items, *i)
		}

		return res, nil
	}
	return nil, errors.New("grpc fromDTO: out is nil")
}

func (cc *GrpcCatalogClient) submit(job job) bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if cc.stopped.Load() {
		return false
	}
	select {
	case cc.jobs <- job:
		return true
	default:
		return false
	}
}

// направляет запрос напрямую
func (cc *GrpcCatalogClient) ForvardRecommend(ctx context.Context, f *catalog.IncomingFeature) (*catalog.Capsule, error) {
	dto, err := toDTO(f)
	if err != nil {
		cc.log.Error("to dto err", "err", err)
		return nil, err
	}

	stream, err := cc.capsuleClient.CapsuleGenerate(ctx, dto)
	if err != nil {
		cc.log.Error("stream err", "err", err)
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
			cc.log.Error("stream Recv err", "err", err)
			return nil, err
		}

		switch v := resp.Payload.(type) {
		case *capsulegen.CapsuleGenerateResponse_Capsule:
			capsule = v.Capsule

		case *capsulegen.CapsuleGenerateResponse_ImageChunk:
			imageData = append(imageData, v.ImageChunk...)

		default:
			cc.log.Error("unknown payload")
		}
	}

	if capsule == nil || len(imageData) == 0 {
		cc.log.Error("nil капсула или картинка")
		return nil, errors.New("nil капсула или картинка")
	}

	res, err := fromDTO(capsule)
	if err != nil {
		cc.log.Error("from dto err")
		return nil, err
	}
	res.Image = imageData
	return res, nil
}

// кладет в канал работ, достает из канала работ, им пользуются сверху, без внутрянки
func (cc *GrpcCatalogClient) Recommend(ctx context.Context, f *catalog.IncomingFeature) (*catalog.Capsule, error) {
	resChan := make(chan capsuleResult, cc.workers) // может тут буфер до 1 уменьшить потом

	job := job{
		id:        cc.increment(),
		capReq:    f,
		ctx:       ctx,
		capResult: resChan,
		jobtype:   getCapsule,
	}

	if ok := cc.submit(job); !ok {
		return nil, errors.New("job не положилась в канал")
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	// case <-time.After(5 * time.Second):
	// return nil, errors.New("capsule timeout")
	case res := <-job.capResult:
		return res.resp, res.err
	}
}
