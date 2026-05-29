package api

import (
	"context"
	"path/filepath"

	"github.com/stashapp/stash/internal/api/loaders"
	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/pkg/models"
)

func (r *folderResolver) Basename(ctx context.Context, obj *models.Folder) (string, error) {
	return filepath.Base(obj.Path), nil
}

// SceneCount resolves the recursive scene count for a folder. It calls the concrete FolderStore
// directly (rather than the FolderReaderWriter interface) so the count method doesn't widen the
// interface or require regenerating mocks. depth is forwarded to the recursive query.
func (r *folderResolver) SceneCount(ctx context.Context, obj *models.Folder, depth *int) (ret int, err error) {
	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		ret, err = manager.GetInstance().Database.Folder.CountScenesInTree(ctx, obj.ID, depth)
		return err
	}); err != nil {
		return 0, err
	}

	return ret, nil
}

// ImageCount resolves the recursive image count for a folder (image counterpart of SceneCount).
func (r *folderResolver) ImageCount(ctx context.Context, obj *models.Folder, depth *int) (ret int, err error) {
	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		ret, err = manager.GetInstance().Database.Folder.CountImagesInTree(ctx, obj.ID, depth)
		return err
	}); err != nil {
		return 0, err
	}

	return ret, nil
}

// TotalSize resolves the recursive total file size (bytes) of a folder's subtree.
func (r *folderResolver) TotalSize(ctx context.Context, obj *models.Folder) (ret int64, err error) {
	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		ret, err = manager.GetInstance().Database.Folder.TotalSizeInTree(ctx, obj.ID)
		return err
	}); err != nil {
		return 0, err
	}

	return ret, nil
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
