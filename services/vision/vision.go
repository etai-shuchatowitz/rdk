// Package vision is the service that allows you to access various computer vision algorithms
// (like detection, segmentation, tracking, etc) that usually only require a camera or image input.
package vision

import (
	"context"
	"image"
	"sync"

	"github.com/edaniels/golog"
	"github.com/invopop/jsonschema"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	goutils "go.viam.com/utils"
	"go.viam.com/utils/rpc"

	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/config"
	servicepb "go.viam.com/rdk/proto/api/service/vision/v1"
	"go.viam.com/rdk/registry"
	"go.viam.com/rdk/resource"
	"go.viam.com/rdk/rlog"
	"go.viam.com/rdk/robot"
	"go.viam.com/rdk/subtype"
	"go.viam.com/rdk/utils"
	viz "go.viam.com/rdk/vision"
	"go.viam.com/rdk/vision/classification"
	objdet "go.viam.com/rdk/vision/objectdetection"
)

func init() {
	registry.RegisterResourceSubtype(Subtype, registry.ResourceSubtype{
		RegisterRPCService: func(ctx context.Context, rpcServer rpc.Server, subtypeSvc subtype.Service) error {
			return rpcServer.RegisterServiceServer(
				ctx,
				&servicepb.VisionService_ServiceDesc,
				NewServer(subtypeSvc),
				servicepb.RegisterVisionServiceHandlerFromEndpoint,
			)
		},
		RPCServiceDesc: &servicepb.VisionService_ServiceDesc,
		RPCClient: func(ctx context.Context, conn rpc.ClientConn, name string, logger golog.Logger) interface{} {
			return NewClientFromConn(ctx, conn, name, logger)
		},
		Reconfigurable: WrapWithReconfigurable,
	})
	registry.RegisterService(Subtype, registry.Service{
		MaxInstance: resource.DefaultMaxInstance,
		Constructor: func(ctx context.Context, r robot.Robot, c config.Service, logger golog.Logger) (interface{}, error) {
			return New(ctx, r, c, logger)
		},
	})
	cType := config.ServiceType(SubtypeName)
	config.RegisterServiceAttributeMapConverter(cType, func(attributeMap config.AttributeMap) (interface{}, error) {
		var attrs Attributes
		return config.TransformAttributeMapToStruct(&attrs, attributeMap)
	},
		&Attributes{},
	)

	resource.AddDefaultService(Named(resource.DefaultServiceName))
}

// A Service that implements various computer vision algorithms like detection and segmentation.
type Service interface {
	// model parameters
	GetModelParameterSchema(ctx context.Context, modelType VisModelType) (*jsonschema.Schema, error)
	// detector methods
	GetDetectorNames(ctx context.Context) ([]string, error)
	AddDetector(ctx context.Context, cfg VisModelConfig) error
	RemoveDetector(ctx context.Context, detectorName string) error
	GetDetectionsFromCamera(ctx context.Context, cameraName, detectorName string) ([]objdet.Detection, error)
	GetDetections(ctx context.Context, img image.Image, detectorName string) ([]objdet.Detection, error)
	// classifier methods
	GetClassifierNames(ctx context.Context) ([]string, error)
	AddClassifier(ctx context.Context, cfg VisModelConfig) error
	RemoveClassifier(ctx context.Context, classifierName string) error
	GetClassificationsFromCamera(ctx context.Context, cameraName, classifierName string, n int) (classification.Classifications, error)
	GetClassifications(ctx context.Context, img image.Image, classifierName string, n int) (classification.Classifications, error)
	// segmenter methods
	GetSegmenterNames(ctx context.Context) ([]string, error)
	AddSegmenter(ctx context.Context, cfg VisModelConfig) error
	RemoveSegmenter(ctx context.Context, segmenterName string) error
	GetObjectPointClouds(ctx context.Context, cameraName, segmenterName string) ([]*viz.Object, error)
}

var (
	_ = Service(&reconfigurableVision{})
	_ = resource.Reconfigurable(&reconfigurableVision{})
	_ = goutils.ContextCloser(&reconfigurableVision{})
)

// NewUnimplementedInterfaceError is used when there is a failed interface check.
func NewUnimplementedInterfaceError(actual interface{}) error {
	return utils.NewUnimplementedInterfaceError((Service)(nil), actual)
}

// SubtypeName is the name of the type of service.
const SubtypeName = resource.SubtypeName("vision")

// RadiusClusteringSegmenter is  the name of a segmenter that finds well separated objects on a flat plane.
const RadiusClusteringSegmenter = "radius_clustering"

// Subtype is a constant that identifies the vision service resource subtype.
var Subtype = resource.NewSubtype(
	resource.ResourceNamespaceRDK,
	resource.ResourceTypeService,
	SubtypeName,
)

// Named is a helper for getting the named vision's typed resource name.
// RSDK-347 Implements vision's Named.
func Named(name string) resource.Name {
	return resource.NameFromSubtype(Subtype, name)
}

// FromRobot is a helper for getting the named vision service from the given Robot.
func FromRobot(r robot.Robot, name string) (Service, error) {
	resource, err := r.ResourceByName(Named(name))
	if err != nil {
		return nil, utils.NewResourceNotFoundError(Named(name))
	}
	svc, ok := resource.(Service)
	if !ok {
		return nil, NewUnimplementedInterfaceError(resource)
	}
	return svc, nil
}

// FindFirstName returns name of first vision service found.
func FindFirstName(r robot.Robot) string {
	for _, val := range robot.NamesBySubtype(r, Subtype) {
		return val
	}
	return ""
}

// FirstFromRobot returns the first vision service in this robot.
func FirstFromRobot(r robot.Robot) (Service, error) {
	name := FindFirstName(r)
	return FromRobot(r, name)
}

// Attributes contains a list of the user-provided details necessary to register a new vision service.
type Attributes struct {
	ModelRegistry []VisModelConfig `json:"register_models"`
}

// New registers new detectors from the config and returns a new object detection service for the given robot.
func New(ctx context.Context, r robot.Robot, config config.Service, logger golog.Logger) (Service, error) {
	modMap := make(modelMap)
	// register detectors and user defined things if config is defined
	if config.ConvertedAttributes != nil {
		attrs, ok := config.ConvertedAttributes.(*Attributes)
		if !ok {
			return nil, utils.NewUnexpectedTypeError(attrs, config.ConvertedAttributes)
		}
		err := registerNewVisModels(ctx, modMap, attrs, logger)
		if err != nil {
			return nil, err
		}
	}
	service := &visionService{
		r:      r,
		modReg: modMap,
		logger: logger,
	}
	return service, nil
}

type visionService struct {
	r      robot.Robot
	modReg modelMap
	logger golog.Logger
}

// GetModelParameterSchema takes the model name and returns the parameters needed to add one to the vision registry.
func (vs *visionService) GetModelParameterSchema(ctx context.Context, modelType VisModelType) (*jsonschema.Schema, error) {
	if modelSchema, ok := RegisteredModelParameterSchemas[modelType]; ok {
		if modelSchema == nil {
			return nil, errors.Errorf("do not have a schema for model type %q", modelType)
		}
		return modelSchema, nil
	}
	return nil, errors.Errorf("do not have a schema for model type %q", modelType)
}

// Detection Methods
// GetDetectorNames returns a list of the all the names of the detectors in the registry.
func (vs *visionService) GetDetectorNames(ctx context.Context) ([]string, error) {
	_, span := trace.StartSpan(ctx, "service::vision::GetDetectorNames")
	defer span.End()
	return vs.modReg.DetectorNames(), nil
}

// AddDetector adds a new detector from an Attribute config struct.
func (vs *visionService) AddDetector(ctx context.Context, cfg VisModelConfig) error {
	ctx, span := trace.StartSpan(ctx, "service::vision::AddDetector")
	defer span.End()
	attrs := &Attributes{ModelRegistry: []VisModelConfig{cfg}}
	err := registerNewVisModels(ctx, vs.modReg, attrs, vs.logger)
	if err != nil {
		return err
	}
	// automatically add a segmenter version of detector
	segmenterConf := VisModelConfig{
		Name:       cfg.Name + "_segmenter",
		Type:       string(DetectorSegmenter),
		Parameters: config.AttributeMap{"detector_name": cfg.Name, "mean_k": 0, "sigma": 0.0},
	}
	return vs.AddSegmenter(ctx, segmenterConf)
}

// RemoveDetector removes a detector from the registry.
func (vs *visionService) RemoveDetector(ctx context.Context, detectorName string) error {
	_, span := trace.StartSpan(ctx, "service::vision::RemoveDetector")
	defer span.End()
	err := vs.modReg.removeVisModel(detectorName, vs.logger)
	if err != nil {
		return err
	}
	// remove the associated segmenter as well (if there is one)
	return vs.RemoveSegmenter(ctx, detectorName+"_segmenter")
}

// GetDetectionsFromCamera returns the detections of the next image from the given camera and the given detector.
func (vs *visionService) GetDetectionsFromCamera(ctx context.Context, cameraName, detectorName string) ([]objdet.Detection, error) {
	ctx, span := trace.StartSpan(ctx, "service::vision::GetDetectionsFromCamera")
	defer span.End()
	cam, err := camera.FromRobot(vs.r, cameraName)
	if err != nil {
		return nil, err
	}
	d, err := vs.modReg.modelLookup(detectorName)
	if err != nil {
		return nil, err
	}
	detector, err := d.toDetector()
	if err != nil {
		return nil, err
	}
	img, release, err := camera.ReadImage(ctx, cam)
	if err != nil {
		return nil, err
	}
	defer release()

	return detector(ctx, img)
}

// GetDetections returns the detections of given image using the given detector.
func (vs *visionService) GetDetections(ctx context.Context, img image.Image, detectorName string,
) ([]objdet.Detection, error) {
	ctx, span := trace.StartSpan(ctx, "service::vision::GetDetections")
	defer span.End()

	d, err := vs.modReg.modelLookup(detectorName)
	if err != nil {
		return nil, err
	}
	detector, err := d.toDetector()
	if err != nil {
		return nil, err
	}

	return detector(ctx, img)
}

// GetClassifierNames returns a list of the all the names of the classifiers in the registry.
func (vs *visionService) GetClassifierNames(ctx context.Context) ([]string, error) {
	_, span := trace.StartSpan(ctx, "service::vision::GetClassifierNames")
	defer span.End()
	return vs.modReg.ClassifierNames(), nil
}

// AddClassifier adds a new classifier from an Attribute config struct.
func (vs *visionService) AddClassifier(ctx context.Context, cfg VisModelConfig) error {
	ctx, span := trace.StartSpan(ctx, "service::vision::AddClassifier")
	defer span.End()
	attrs := &Attributes{ModelRegistry: []VisModelConfig{cfg}}
	err := registerNewVisModels(ctx, vs.modReg, attrs, vs.logger)
	if err != nil {
		return err
	}
	return nil
}

// Remove classifier removes a classifier from the registry.
func (vs *visionService) RemoveClassifier(ctx context.Context, classifierName string) error {
	_, span := trace.StartSpan(ctx, "service::vision::RemoveClassifier")
	defer span.End()
	err := vs.modReg.removeVisModel(classifierName, vs.logger)
	if err != nil {
		return err
	}
	return nil
}

// GetClassificationsFromCamera returns the classifications of the next image from the given camera and the given detector.
func (vs *visionService) GetClassificationsFromCamera(ctx context.Context, cameraName,
	classifierName string, n int,
) (classification.Classifications, error) {
	ctx, span := trace.StartSpan(ctx, "service::vision::GetClassificationsFromCamera")
	defer span.End()
	cam, err := camera.FromRobot(vs.r, cameraName)
	if err != nil {
		return nil, err
	}
	c, err := vs.modReg.modelLookup(classifierName)
	if err != nil {
		return nil, err
	}
	classifier, err := c.toClassifier()
	if err != nil {
		return nil, err
	}
	img, release, err := camera.ReadImage(ctx, cam)
	if err != nil {
		return nil, err
	}
	defer release()
	fullClassifications, err := classifier(ctx, img)
	if err != nil {
		return nil, err
	}
	return fullClassifications.TopN(n)
}

// GetClassifications returns the classifications of given image using the given classifier.
func (vs *visionService) GetClassifications(ctx context.Context, img image.Image,
	classifierName string, n int,
) (classification.Classifications, error) {
	ctx, span := trace.StartSpan(ctx, "service::vision::GetClassifications")
	defer span.End()

	c, err := vs.modReg.modelLookup(classifierName)
	if err != nil {
		return nil, err
	}
	classifier, err := c.toClassifier()
	if err != nil {
		return nil, err
	}
	fullClassifications, err := classifier(ctx, img)
	if err != nil {
		return nil, err
	}
	return fullClassifications.TopN(n)
}

// Segmentation Methods
// GetSegmenterNames returns a list of all the names of the segmenters in the segmenter map.
func (vs *visionService) GetSegmenterNames(ctx context.Context) ([]string, error) {
	_, span := trace.StartSpan(ctx, "service::vision::GetSegmenterNames")
	defer span.End()
	return vs.modReg.SegmenterNames(), nil
}

// AddSegmenter adds a new segmenter from an Attribute config struct.
func (vs *visionService) AddSegmenter(ctx context.Context, cfg VisModelConfig) error {
	ctx, span := trace.StartSpan(ctx, "service::vision::AddSegmenter")
	defer span.End()
	attrs := &Attributes{ModelRegistry: []VisModelConfig{cfg}}
	return registerNewVisModels(ctx, vs.modReg, attrs, vs.logger)
}

// RemoveSegmenter removes a segmenter from the registry.
func (vs *visionService) RemoveSegmenter(ctx context.Context, segmenterName string) error {
	_, span := trace.StartSpan(ctx, "service::vision::RemoveSegmenter")
	defer span.End()
	return vs.modReg.removeVisModel(segmenterName, vs.logger)
}

// GetObjectPointClouds returns all the found objects in a 3D image according to the chosen segmenter.
func (vs *visionService) GetObjectPointClouds(
	ctx context.Context,
	cameraName string,
	segmenterName string,
) ([]*viz.Object, error) {
	ctx, span := trace.StartSpan(ctx, "service::vision::GetObjectPointClouds")
	defer span.End()
	cam, err := camera.FromRobot(vs.r, cameraName)
	if err != nil {
		return nil, err
	}
	s, err := vs.modReg.modelLookup(segmenterName)
	if err != nil {
		return nil, err
	}
	segmenter, err := s.toSegmenter()
	if err != nil {
		return nil, err
	}
	return segmenter(ctx, cam)
}

// Close removes all existing detectors from the vision service.
func (vs *visionService) Close() error {
	models := vs.modReg.modelNames()
	for _, detectorName := range models {
		err := vs.modReg.removeVisModel(detectorName, vs.logger)
		if err != nil {
			return err
		}
	}
	return nil
}

type reconfigurableVision struct {
	mu     sync.RWMutex
	actual Service
}

func (svc *reconfigurableVision) GetModelParameterSchema(ctx context.Context, modelType VisModelType) (*jsonschema.Schema, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.actual.GetModelParameterSchema(ctx, modelType)
}

func (svc *reconfigurableVision) GetDetectorNames(ctx context.Context) ([]string, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.actual.GetDetectorNames(ctx)
}

func (svc *reconfigurableVision) AddDetector(ctx context.Context, cfg VisModelConfig) error {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.actual.AddDetector(ctx, cfg)
}

func (svc *reconfigurableVision) RemoveDetector(ctx context.Context, detectorName string) error {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.actual.RemoveDetector(ctx, detectorName)
}

func (svc *reconfigurableVision) GetDetectionsFromCamera(ctx context.Context, cameraName, detectorName string) ([]objdet.Detection, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.actual.GetDetectionsFromCamera(ctx, cameraName, detectorName)
}

func (svc *reconfigurableVision) GetDetections(ctx context.Context, img image.Image, detectorName string,
) ([]objdet.Detection, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.actual.GetDetections(ctx, img, detectorName)
}

func (svc *reconfigurableVision) GetClassifierNames(ctx context.Context) ([]string, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.actual.GetClassifierNames(ctx)
}

func (svc *reconfigurableVision) AddClassifier(ctx context.Context, cfg VisModelConfig) error {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.actual.AddClassifier(ctx, cfg)
}

func (svc *reconfigurableVision) RemoveClassifier(ctx context.Context, classifierName string) error {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.actual.RemoveDetector(ctx, classifierName)
}

func (svc *reconfigurableVision) GetClassificationsFromCamera(ctx context.Context, cameraName,
	classifierName string, n int,
) (classification.Classifications, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.actual.GetClassificationsFromCamera(ctx, cameraName, classifierName, n)
}

func (svc *reconfigurableVision) GetClassifications(ctx context.Context, img image.Image,
	classifierName string, n int,
) (classification.Classifications, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.actual.GetClassifications(ctx, img, classifierName, n)
}

func (svc *reconfigurableVision) GetSegmenterNames(ctx context.Context) ([]string, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.actual.GetSegmenterNames(ctx)
}

func (svc *reconfigurableVision) AddSegmenter(ctx context.Context, cfg VisModelConfig) error {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.actual.AddSegmenter(ctx, cfg)
}

func (svc *reconfigurableVision) RemoveSegmenter(ctx context.Context, segmenterName string) error {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.actual.RemoveSegmenter(ctx, segmenterName)
}

func (svc *reconfigurableVision) GetObjectPointClouds(ctx context.Context,
	cameraName,
	segmenterName string,
) ([]*viz.Object, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return svc.actual.GetObjectPointClouds(ctx, cameraName, segmenterName)
}

func (svc *reconfigurableVision) Close(ctx context.Context) error {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	return goutils.TryClose(ctx, svc.actual)
}

// Reconfigure replaces the old vision service with a new vision.
func (svc *reconfigurableVision) Reconfigure(ctx context.Context, newSvc resource.Reconfigurable) error {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rSvc, ok := newSvc.(*reconfigurableVision)
	if !ok {
		return utils.NewUnexpectedTypeError(svc, newSvc)
	}
	if err := goutils.TryClose(ctx, svc.actual); err != nil {
		rlog.Logger.Errorw("error closing old", "error", err)
	}
	svc.actual = rSvc.actual
	/*
		theOldServ := svc.actual.(*visionService)
		theNewSerc := rSvc.actual.(*visionService)
		*theOldServ = *theNewSerc
	*/
	return nil
}

// WrapWithReconfigurable wraps a vision service as a Reconfigurable.
func WrapWithReconfigurable(s interface{}) (resource.Reconfigurable, error) {
	svc, ok := s.(Service)
	if !ok {
		return nil, NewUnimplementedInterfaceError(s)
	}

	if reconfigurable, ok := s.(*reconfigurableVision); ok {
		return reconfigurable, nil
	}

	return &reconfigurableVision{actual: svc}, nil
}
