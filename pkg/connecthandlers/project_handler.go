package connecthandlers

import (
	"context"

	connect "connectrpc.com/connect"
	typespb "github.com/candelahq/candela/gen/go/candela/types"
	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/pkg/storage"
	"github.com/candelahq/candela/pkg/storage/projectdb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ProjectHandler implements the ProjectService ConnectRPC handler.
type ProjectHandler struct {
	store storage.ProjectStore
}

// NewProjectHandler creates a new ProjectHandler.
func NewProjectHandler(store storage.ProjectStore) *ProjectHandler {
	return &ProjectHandler{store: store}
}

func (h *ProjectHandler) CreateProject(
	ctx context.Context,
	req *connect.Request[v1.CreateProjectRequest],
) (*connect.Response[v1.CreateProjectResponse], error) {
	p, err := h.store.CreateProject(ctx, storage.Project{
		Name:        req.Msg.Name,
		Description: req.Msg.Description,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&v1.CreateProjectResponse{
		Project: projectToProto(p),
	}), nil
}

func (h *ProjectHandler) GetProject(
	ctx context.Context,
	req *connect.Request[v1.GetProjectRequest],
) (*connect.Response[v1.GetProjectResponse], error) {
	p, err := h.store.GetProject(ctx, req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	return connect.NewResponse(&v1.GetProjectResponse{
		Project: projectToProto(p),
	}), nil
}

func (h *ProjectHandler) ListProjects(
	ctx context.Context,
	req *connect.Request[v1.ListProjectsRequest],
) (*connect.Response[v1.ListProjectsResponse], error) {
	limit := 50
	offset := 0
	if req.Msg.Pagination != nil {
		if req.Msg.Pagination.PageSize > 0 {
			limit = int(req.Msg.Pagination.PageSize)
		}
	}

	projects, total, err := h.store.ListProjects(ctx, limit, offset)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	pbProjects := make([]*typespb.Project, len(projects))
	for i, p := range projects {
		pp := p // avoid capture
		pbProjects[i] = projectToProto(&pp)
	}

	return connect.NewResponse(&v1.ListProjectsResponse{
		Projects: pbProjects,
		Pagination: &typespb.PaginationResponse{
			TotalCount: int32(total),
		},
	}), nil
}

func (h *ProjectHandler) DeleteProject(
	ctx context.Context,
	req *connect.Request[v1.DeleteProjectRequest],
) (*connect.Response[v1.DeleteProjectResponse], error) {
	if err := h.store.DeleteProject(ctx, req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&v1.DeleteProjectResponse{}), nil
}

func (h *ProjectHandler) CreateAPIKey(
	ctx context.Context,
	req *connect.Request[v1.CreateAPIKeyRequest],
) (*connect.Response[v1.CreateAPIKeyResponse], error) {
	fullKey := projectdb.GenerateAPIKey()

	key, err := h.store.CreateAPIKey(ctx, storage.APIKey{
		ProjectID: req.Msg.ProjectId,
		Name:      req.Msg.Name,
	}, fullKey)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&v1.CreateAPIKeyResponse{
		ApiKey:  apiKeyToProto(key),
		FullKey: fullKey,
	}), nil
}

func (h *ProjectHandler) ListAPIKeys(
	ctx context.Context,
	req *connect.Request[v1.ListAPIKeysRequest],
) (*connect.Response[v1.ListAPIKeysResponse], error) {
	keys, err := h.store.ListAPIKeys(ctx, req.Msg.ProjectId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	pbKeys := make([]*typespb.APIKey, len(keys))
	for i, k := range keys {
		kk := k
		pbKeys[i] = apiKeyToProto(&kk)
	}

	return connect.NewResponse(&v1.ListAPIKeysResponse{
		ApiKeys: pbKeys,
	}), nil
}

func (h *ProjectHandler) RevokeAPIKey(
	ctx context.Context,
	req *connect.Request[v1.RevokeAPIKeyRequest],
) (*connect.Response[v1.RevokeAPIKeyResponse], error) {
	if err := h.store.RevokeAPIKey(ctx, req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&v1.RevokeAPIKeyResponse{}), nil
}

// --- Proto converters ---

func projectToProto(p *storage.Project) *typespb.Project {
	return &typespb.Project{
		Id:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		CreatedAt:   timestamppb.New(p.CreatedAt),
		UpdatedAt:   timestamppb.New(p.UpdatedAt),
	}
}

func apiKeyToProto(k *storage.APIKey) *typespb.APIKey {
	key := &typespb.APIKey{
		Id:        k.ID,
		ProjectId: k.ProjectID,
		Name:      k.Name,
		KeyPrefix: k.KeyPrefix,
		Active:    k.Active,
		CreatedAt: timestamppb.New(k.CreatedAt),
	}
	if !k.ExpiresAt.IsZero() {
		key.ExpiresAt = timestamppb.New(k.ExpiresAt)
	}
	return key
}
