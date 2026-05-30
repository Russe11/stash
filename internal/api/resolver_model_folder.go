package api

import (
	"context"
	"path/filepath"

	"github.com/stashapp/stash/internal/api/loaders"
	"github.com/stashapp/stash/pkg/models"
)

func (r *folderResolver) Basename(ctx context.Context, obj *models.Folder) (string, error) {
	return filepath.Base(obj.Path), nil
}

// SceneCount resolves the recursive scene count for a folder. A folder-list view resolves this field
// for every row, so it goes through a dataloader (keyed by folder + depth) that coalesces those into
// one windowed query instead of one recursive CTE per folder. depth is normalized so all "unlimited"
// callers share a cache key.
func (r *folderResolver) SceneCount(ctx context.Context, obj *models.Folder, depth *int) (int, error) {
	return loaders.From(ctx).FolderSceneCount.Load(loaders.FolderCountKey{
		FolderID: obj.ID,
		Depth:    loaders.NormalizeFolderDepth(depth),
	})
}

// ImageCount resolves the recursive image count for a folder (image counterpart of SceneCount).
func (r *folderResolver) ImageCount(ctx context.Context, obj *models.Folder, depth *int) (int, error) {
	return loaders.From(ctx).FolderImageCount.Load(loaders.FolderCountKey{
		FolderID: obj.ID,
		Depth:    loaders.NormalizeFolderDepth(depth),
	})
}

// TotalSize resolves the recursive total file size (bytes) of a folder's subtree, batched per request
// (recursion is always unlimited, so the loader is keyed by folder id alone).
func (r *folderResolver) TotalSize(ctx context.Context, obj *models.Folder) (int64, error) {
	return loaders.From(ctx).FolderTotalSize.Load(obj.ID)
}

func (r *folderResolver) ParentFolder(ctx context.Context, obj *models.Folder) (*models.Folder, error) {
	if obj.ParentFolderID == nil {
		return nil, nil
	}

	if r.idOnly(ctx) {
		return &models.Folder{ID: *obj.ParentFolderID}, nil
	}

	return loaders.From(ctx).FolderByID.Load(*obj.ParentFolderID)
}

func foldersFromIDs(ids []models.FolderID) []*models.Folder {
	ret := make([]*models.Folder, len(ids))
	for i, id := range ids {
		ret[i] = &models.Folder{ID: id}
	}
	return ret
}

func (r *folderResolver) ParentFolders(ctx context.Context, obj *models.Folder) ([]*models.Folder, error) {
	ids, err := loaders.From(ctx).FolderParentFolderIDs.Load(obj.ID)
	if err != nil {
		return nil, err
	}

	if r.idOnly(ctx) {
		return foldersFromIDs(ids), nil
	}

	var errs []error
	ret, errs := loaders.From(ctx).FolderByID.LoadAll(ids)
	return ret, firstError(errs)
}

func (r *folderResolver) SubFolders(ctx context.Context, obj *models.Folder) ([]*models.Folder, error) {
	ids, err := loaders.From(ctx).FolderSubFolderIDs.Load(obj.ID)
	if err != nil {
		return nil, err
	}

	if r.idOnly(ctx) {
		return foldersFromIDs(ids), nil
	}

	var errs []error
	ret, errs := loaders.From(ctx).FolderByID.LoadAll(ids)
	return ret, firstError(errs)
}

func (r *folderResolver) ZipFile(ctx context.Context, obj *models.Folder) (*BasicFile, error) {
	// shortcut for id only queries
	if r.idOnly(ctx) {
		if obj.ZipFileID == nil {
			return nil, nil
		}

		return &BasicFile{
			BaseFile: &models.BaseFile{ID: *obj.ZipFileID},
		}, nil
	}

	return zipFileResolver(ctx, obj.ZipFileID)
}
