package sensor

import (
	"context"
	"errors"
	"fmt"

	pb "go.viam.com/api/common/v1"
	"google.golang.org/protobuf/types/known/anypb"

	"go.viam.com/rdk/data"
	"go.viam.com/rdk/protoutils"
)

type method int64

const (
	readings method = iota
)

func (m method) String() string {
	if m == readings {
		return "Readings"
	}
	return "Unknown"
}

// newReadingsCollector returns a collector to register a sensor reading method. If one is already registered
// with the same MethodMetadata it will panic.
func newReadingsCollector(resource interface{}, params data.CollectorParams, tagger data.Tagger) (data.Collector, error) {
	sensorResource, err := assertSensor(resource)
	if err != nil {
		return nil, err
	}

	cFunc := data.CaptureFunc(func(ctx context.Context, arg map[string]*anypb.Any, tagger data.Tagger) (interface{}, []string, error) {
		values, err := sensorResource.Readings(ctx, data.FromDMExtraMap)

		tags := tagger.Tags // this would actually be tagger.GetTags()

		if err != nil {
			// A modular filter component can be created to filter the readings from a component. The error ErrNoCaptureToStore
			// is used in the datamanager to exclude readings from being captured and stored.
			if errors.Is(err, data.ErrNoCaptureToStore) {
				return nil, nil, err
			}

			return nil, nil, data.FailedToReadErr(params.ComponentName, readings.String(), err)
		}
		readings, err := protoutils.ReadingGoToProto(values)
		if err != nil {
			return nil, nil, err
		}
		fmt.Printf("\n---tags: %s---\n", tags)
		return pb.GetReadingsResponse{
			Readings: readings,
		}, tags, nil
	})
	return data.NewCollector(cFunc, params)
}

func assertSensor(resource interface{}) (Sensor, error) {
	sensorResource, ok := resource.(Sensor)
	if !ok {
		return nil, data.InvalidInterfaceErr(API)
	}

	return sensorResource, nil
}
