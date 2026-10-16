package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"

	adminV1 "framework/api/gen/go/admin/v1"
	fileV1 "framework/api/gen/go/file/v1"

	"framework/pkg/storage"
)

// FileUploadRoute 后端上传接口路由（本地存储模式下前端不再直传对象存储，统一走后端上传）。
const FileUploadRoute = "/api/v1/monitor/file:upload"

type OssService struct {
	adminV1.OssServiceHTTPServer

	log *log.Helper

	store *storage.LocalStorage
}

func NewOssService(logger log.Logger, store *storage.LocalStorage) *OssService {
	l := log.NewHelper(log.With(logger, "module", "oss/service/admin-service"))
	return &OssService{
		log:   l,
		store: store,
	}
}

// OssUploadUrl 返回上传/下载地址（决策 D6：本地存储无预签名概念，
// upload_url 固定指向后端上传接口，download_url 指向鉴权下载接口，客户端携带访问令牌使用）。
func (s *OssService) OssUploadUrl(ctx context.Context, req *fileV1.OssUploadUrlRequest) (*fileV1.OssUploadUrlResponse, error) {
	if s.store == nil {
		return nil, fileV1.ErrorInternalServerError("storage not configured")
	}

	var bucketName string
	if req.BucketName != nil {
		bucketName = req.GetBucketName()
	} else {
		bucketName = s.store.ContentTypeToBucketName(req.GetContentType())
	}

	objectName, _ := s.store.JointObjectName(req.GetContentType(), req.FilePath, req.FileName)

	return &fileV1.OssUploadUrlResponse{
		UploadUrl:   FileUploadRoute,
		DownloadUrl: storage.BuildDownloadURL(bucketName + "/" + objectName),
		ObjectName:  objectName,
		BucketName:  trans.Ptr(bucketName),
	}, nil
}

func (s *OssService) PostUploadFile(ctx context.Context, req *fileV1.UploadOssFileRequest) (*fileV1.UploadOssFileResponse, error) {
	return s.upload(ctx, req)
}

func (s *OssService) PutUploadFile(ctx context.Context, req *fileV1.UploadOssFileRequest) (*fileV1.UploadOssFileResponse, error) {
	return s.upload(ctx, req)
}

func (s *OssService) upload(ctx context.Context, req *fileV1.UploadOssFileRequest) (*fileV1.UploadOssFileResponse, error) {
	if s.store == nil {
		return nil, fileV1.ErrorInternalServerError("storage not configured")
	}
	if req.File == nil {
		return nil, fileV1.ErrorBadRequest("multipart file field is required")
	}

	if req.BucketName == nil {
		req.BucketName = trans.Ptr(s.store.ContentTypeToBucketName(req.GetMime()))
	}
	if req.ObjectName == nil {
		req.ObjectName = trans.Ptr(req.GetSourceFileName())
	}

	relPath, err := s.store.Upload(ctx, req.GetBucketName(), req.GetObjectName(), req.GetFile())
	if err != nil {
		return nil, fileV1.ErrorUploadFailed("upload failed: %s", err.Error())
	}

	return &fileV1.UploadOssFileResponse{
		Url: storage.BuildDownloadURL(relPath),
	}, nil
}
