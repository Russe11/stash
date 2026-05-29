package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/jmoiron/sqlx"
	"github.com/stashapp/stash/pkg/models"
	"gopkg.in/guregu/null.v4"
)

const folderTable = "folders"
const folderIDColumn = "folder_id"

type folderRow struct {
	ID             models.FolderID `db:"id" goqu:"skipinsert"`
	Basename       string          `db:"basename"`
	Path           string          `db:"path"`
	ZipFileID      null.Int        `db:"zip_file_id"`
	ParentFolderID null.Int        `db:"parent_folder_id"`
	ModTime        Timestamp       `db:"mod_time"`
	CreatedAt      Timestamp       `db:"created_at"`
	UpdatedAt      Timestamp       `db:"updated_at"`
}

func (r *folderRow) fromFolder(o models.Folder) {
	r.ID = o.ID
	// derive basename from path
	r.Basename = filepath.Base(o.Path)
	r.Path = o.Path
	r.ZipFileID = nullIntFromFileIDPtr(o.ZipFileID)
	r.ParentFolderID = nullIntFromFolderIDPtr(o.ParentFolderID)
	r.ModTime = Timestamp{Timestamp: o.ModTime}
	r.CreatedAt = Timestamp{Timestamp: o.CreatedAt}
	r.UpdatedAt = Timestamp{Timestamp: o.UpdatedAt}
}

type folderQueryRow struct {
	folderRow

	ZipBasename   null.String `db:"zip_basename"`
	ZipFolderPath null.String `db:"zip_folder_path"`
	ZipSize       null.Int    `db:"zip_size"`
}

func (r *folderQueryRow) resolve() *models.Folder {
	ret := &models.Folder{
		ID: r.ID,
		DirEntry: models.DirEntry{
			ZipFileID: nullIntFileIDPtr(r.ZipFileID),
			ModTime:   r.ModTime.Timestamp,
		},
		Path:           string(r.Path),
		ParentFolderID: nullIntFolderIDPtr(r.ParentFolderID),
		CreatedAt:      r.CreatedAt.Timestamp,
		UpdatedAt:      r.UpdatedAt.Timestamp,
	}

	if ret.ZipFileID != nil && r.ZipFolderPath.Valid && r.ZipBasename.Valid {
		ret.ZipFile = &models.BaseFile{
			ID:       *ret.ZipFileID,
			Path:     filepath.Join(r.ZipFolderPath.String, r.ZipBasename.String),
			Basename: r.ZipBasename.String,
			Size:     r.ZipSize.Int64,
		}
	}

	return ret
}

type folderQueryRows []folderQueryRow

func (r folderQueryRows) resolve() []*models.Folder {
	var ret []*models.Folder

	for _, row := range r {
		f := row.resolve()
		ret = append(ret, f)
	}

	return ret
}

type folderRepositoryType struct {
	repository

	galleries repository
}

var (
	folderRepository = folderRepositoryType{
		repository: repository{
			tableName: folderTable,
			idColumn:  idColumn,
		},
		galleries: repository{
			tableName: galleryTable,
			idColumn:  folderIDColumn,
		},
	}
)

type FolderStore struct {
	repository

	tableMgr *table
}

func NewFolderStore() *FolderStore {
	return &FolderStore{
		repository: repository{
			tableName: folderTable,
			idColumn:  idColumn,
		},

		tableMgr: folderTableMgr,
	}
}

func (qb *FolderStore) Create(ctx context.Context, f *models.Folder) error {
	var r folderRow
	r.fromFolder(*f)

	id, err := qb.tableMgr.insertID(ctx, r)
	if err != nil {
		return err
	}

	// only assign id once we are successful
	f.ID = models.FolderID(id)

	return nil
}

func (qb *FolderStore) Update(ctx context.Context, updatedObject *models.Folder) error {
	var r folderRow
	r.fromFolder(*updatedObject)

	if err := qb.tableMgr.updateByID(ctx, updatedObject.ID, r); err != nil {
		return err
	}

	return nil
}

func (qb *FolderStore) Destroy(ctx context.Context, id models.FolderID) error {
	return qb.tableMgr.destroyExisting(ctx, []int{int(id)})
}

func (qb *FolderStore) table() exp.IdentifierExpression {
	return qb.tableMgr.table
}

func (qb *FolderStore) selectDataset() *goqu.SelectDataset {
	table := qb.table()
	fileTable := fileTableMgr.table

	zipFileTable := fileTable.As("zip_files")
	zipFolderTable := table.As("zip_files_folders")

	cols := []interface{}{
		table.Col("id"),
		table.Col("path"),
		table.Col("zip_file_id"),
		table.Col("parent_folder_id"),
		table.Col("mod_time"),
		table.Col("created_at"),
		table.Col("updated_at"),
		zipFileTable.Col("basename").As("zip_basename"),
		zipFolderTable.Col("path").As("zip_folder_path"),
		// size is needed to open containing zip files
		zipFileTable.Col("size").As("zip_size"),
	}

	ret := dialect.From(table).Select(cols...)

	return ret.LeftJoin(
		zipFileTable,
		goqu.On(table.Col("zip_file_id").Eq(zipFileTable.Col("id"))),
	).LeftJoin(
		zipFolderTable,
		goqu.On(zipFileTable.Col("parent_folder_id").Eq(zipFolderTable.Col(idColumn))),
	)
}

func (qb *FolderStore) countDataset() *goqu.SelectDataset {
	table := qb.table()
	fileTable := fileTableMgr.table

	zipFileTable := fileTable.As("zip_files")
	zipFolderTable := table.As("zip_files_folders")

	ret := dialect.From(table).Select(goqu.COUNT(goqu.DISTINCT(table.Col("id"))))

	return ret.LeftJoin(
		zipFileTable,
		goqu.On(table.Col("zip_file_id").Eq(zipFileTable.Col("id"))),
	).LeftJoin(
		zipFolderTable,
		goqu.On(zipFileTable.Col("parent_folder_id").Eq(zipFolderTable.Col(idColumn))),
	)
}

func (qb *FolderStore) get(ctx context.Context, q *goqu.SelectDataset) (*models.Folder, error) {
	ret, err := qb.getMany(ctx, q)
	if err != nil {
		return nil, err
	}

	if len(ret) == 0 {
		return nil, sql.ErrNoRows
	}

	return ret[0], nil
}

func (qb *FolderStore) getMany(ctx context.Context, q *goqu.SelectDataset) ([]*models.Folder, error) {
	const single = false
	var rows folderQueryRows
	if err := queryFunc(ctx, q, single, func(r *sqlx.Rows) error {
		var f folderQueryRow
		if err := r.StructScan(&f); err != nil {
			return err
		}

		rows = append(rows, f)
		return nil
	}); err != nil {
		return nil, err
	}

	return rows.resolve(), nil
}

func (qb *FolderStore) Find(ctx context.Context, id models.FolderID) (*models.Folder, error) {
	q := qb.selectDataset().Where(qb.tableMgr.byID(id))

	ret, err := qb.get(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("getting folder by id %d: %w", id, err)
	}

	return ret, nil
}

// FindByIDs finds multiple folders by their IDs.
// No check is made to see if the folders exist, and the order of the returned folders
// is not guaranteed to be the same as the order of the input IDs.
func (qb *FolderStore) FindByIDs(ctx context.Context, ids []models.FolderID) ([]*models.Folder, error) {
	folders := make([]*models.Folder, 0, len(ids))

	table := qb.table()
	if err := batchExec(ids, defaultBatchSize, func(batch []models.FolderID) error {
		q := qb.selectDataset().Prepared(true).Where(table.Col(idColumn).In(batch))
		unsorted, err := qb.getMany(ctx, q)
		if err != nil {
			return err
		}

		folders = append(folders, unsorted...)

		return nil
	}); err != nil {
		return nil, err
	}

	return folders, nil
}

func (qb *FolderStore) FindMany(ctx context.Context, ids []models.FolderID) ([]*models.Folder, error) {
	folders := make([]*models.Folder, len(ids))

	unsorted, err := qb.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	for _, s := range unsorted {
		i := slices.Index(ids, s.ID)
		folders[i] = s
	}

	for i := range folders {
		if folders[i] == nil {
			return nil, fmt.Errorf("folder with id %d not found", ids[i])
		}
	}

	return folders, nil
}

func (qb *FolderStore) FindByPath(ctx context.Context, p string, caseSensitive bool) (*models.Folder, error) {
	// use like for case insensitive search
	var criterion exp.BooleanExpression
	if caseSensitive {
		criterion = qb.table().Col("path").Eq(p)
	} else {
		criterion = qb.table().Col("path").ILike(p)
	}

	q := qb.selectDataset().Prepared(true).Where(criterion)

	ret, err := qb.get(ctx, q)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("getting folder by path %s: %w", p, err)
	}

	return ret, nil
}

func (qb *FolderStore) FindByParentFolderID(ctx context.Context, parentFolderID models.FolderID) ([]*models.Folder, error) {
	q := qb.selectDataset().Where(qb.table().Col("parent_folder_id").Eq(int(parentFolderID)))

	ret, err := qb.getMany(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("getting folders by parent folder id %d: %w", parentFolderID, err)
	}

	return ret, nil
}

func (qb *FolderStore) GetManyParentFolderIDs(ctx context.Context, folderIDs []models.FolderID) ([][]models.FolderID, error) {
	table := qb.table()

	// SQL recursive query to get all parent folder IDs for each folder ID
	/*
		WITH RECURSIVE parent_folders AS (
		    SELECT id, parent_folder_id
		    FROM folders
		    WHERE id IN (folderIDs)

		    UNION ALL

		    SELECT f.id, f.parent_folder_id
		    FROM folders f
		    INNER JOIN parent_folders pf ON f.id = pf.parent_folder_id
		)
		SELECT id, parent_folder_id FROM parent_folders;
	*/
	const parentFolders = "parent_folders"
	const parentFolderID = "parent_folder_id"
	const parentID = "parent_id"
	const foldersAlias = "f"

	const parentFoldersAlias = "pf"
	foldersAliasedI := table.As(foldersAlias)
	parentFoldersI := goqu.T(parentFolders).As(parentFoldersAlias)

	q := dialect.From(parentFolders).Prepared(true).
		WithRecursive(parentFolders,
			dialect.From(table).Select(table.Col(idColumn), table.Col(parentFolderID).As(parentID)).
				Where(table.Col(idColumn).In(folderIDs)).
				Union(
					dialect.From(foldersAliasedI).InnerJoin(
						parentFoldersI,
						goqu.On(foldersAliasedI.Col(idColumn).Eq(parentFoldersI.Col(parentID))),
					).Select(foldersAliasedI.Col(idColumn), foldersAliasedI.Col(parentFolderID).As(parentID)),
				),
		).Select(idColumn, parentID)

	type resultRow struct {
		FolderID       models.FolderID `db:"id"`
		ParentFolderID null.Int        `db:"parent_id"`
	}

	folderMap := make(map[models.FolderID]models.FolderID)

	if err := queryFunc(ctx, q, false, func(r *sqlx.Rows) error {
		var row resultRow
		if err := r.StructScan(&row); err != nil {
			return err
		}

		if row.ParentFolderID.Valid {
			folderMap[row.FolderID] = models.FolderID(row.ParentFolderID.Int64)
		} else {
			folderMap[row.FolderID] = 0
		}

		return nil
	}); err != nil {
		return nil, err
	}

	ret := make([][]models.FolderID, len(folderIDs))

	for i, folderID := range folderIDs {
		var parents []models.FolderID
		currentID := folderID

		for {
			parentID, exists := folderMap[currentID]
			if !exists || parentID == 0 {
				break
			}
			parents = append(parents, parentID)
			currentID = parentID
		}

		ret[i] = parents
	}

	return ret, nil
}

func (qb *FolderStore) GetManySubFolderIDs(ctx context.Context, parentFolderIDs []models.FolderID) ([][]models.FolderID, error) {
	table := qb.table()
	q := dialect.From(table).Select(
		table.Col(idColumn),
		table.Col("parent_folder_id"),
	).Where(qb.table().Col("parent_folder_id").In(parentFolderIDs))

	sql, args, err := q.ToSQL()
	if err != nil {
		return nil, fmt.Errorf("building query: %w", err)
	}

	var results []struct {
		FolderID       int             `db:"id"`
		ParentFolderID models.FolderID `db:"parent_folder_id"`
	}

	if err := querySelect(ctx, sql, args, &results); err != nil {
		return nil, fmt.Errorf("getting folders by parent folder ids %v: %w", parentFolderIDs, err)
	}

	retMap := make(map[models.FolderID][]models.FolderID)

	for _, v := range results {
		retMap[v.ParentFolderID] = append(retMap[v.ParentFolderID], models.FolderID(v.FolderID))
	}

	ret := make([][]models.FolderID, len(parentFolderIDs))

	for i, parentID := range parentFolderIDs {
		ret[i] = retMap[parentID]
	}

	return ret, nil
}

func (qb *FolderStore) allInPaths(q *goqu.SelectDataset, p []string) *goqu.SelectDataset {
	table := qb.table()

	var conds []exp.Expression
	for _, pp := range p {
		ppWildcard := pp + string(filepath.Separator) + "%"

		conds = append(conds, table.Col("path").Eq(pp), table.Col("path").Like(ppWildcard))
	}

	return q.Where(
		goqu.Or(conds...),
	)
}

// FindAllInPaths returns the all folders that are or are within any of the given paths.
// Returns all if limit is < 0.
// Returns all folders if p is empty.
func (qb *FolderStore) FindAllInPaths(ctx context.Context, p []string, includeZipContents bool, limit, offset int) ([]*models.Folder, error) {
	q := qb.selectDataset().Prepared(true)
	q = qb.allInPaths(q, p)

	if !includeZipContents {
		q = q.Where(qb.table().Col("zip_file_id").IsNull())
	}

	if limit > -1 {
		q = q.Limit(uint(limit))
	}

	q = q.Offset(uint(offset))

	ret, err := qb.getMany(ctx, q)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("getting folders in path %s: %w", p, err)
	}

	return ret, nil
}

// CountScenesInTree returns the number of distinct scenes whose files live in the folder with the
// given id or, recursively, its sub-folders. depth controls recursion using the same convention as
// the rest of the codebase (e.g. group counts): nil or negative = unlimited; 0 = this folder only;
// n > 0 = this folder plus n levels of descendants.
func (qb *FolderStore) CountScenesInTree(ctx context.Context, id models.FolderID, depth *int) (int, error) {
	// limit the recursion depth when requested. depth 0 yields `sub.lvl < 0`, which admits no
	// descendants, leaving only the folder itself.
	levelClause := ""
	args := []interface{}{id}
	if depth != nil && *depth >= 0 {
		levelClause = "WHERE sub.lvl < ?"
		args = append(args, *depth)
	}

	query := fmt.Sprintf(`WITH RECURSIVE sub(id, lvl) AS (
    SELECT id, 0 FROM folders WHERE id = ?
    UNION ALL
    SELECT folders.id, sub.lvl + 1 FROM folders JOIN sub ON folders.parent_folder_id = sub.id %s
)
SELECT COUNT(DISTINCT scenes_files.scene_id)
FROM scenes_files
JOIN files ON files.id = scenes_files.file_id
WHERE files.parent_folder_id IN (SELECT id FROM sub)`, levelClause)

	wrapper := dbWrapperType{}
	rows, err := wrapper.QueryxContext(ctx, query, args...)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("counting scenes in folder tree %d: %w", id, err)
	}
	defer rows.Close()

	var count int
	if rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	return count, nil
}

// CountImagesInTree is the image counterpart of CountScenesInTree: distinct images whose files live
// in the folder or, per depth, its sub-folders (nil/<0 unlimited, 0 self, n levels).
func (qb *FolderStore) CountImagesInTree(ctx context.Context, id models.FolderID, depth *int) (int, error) {
	levelClause := ""
	args := []interface{}{id}
	if depth != nil && *depth >= 0 {
		levelClause = "WHERE sub.lvl < ?"
		args = append(args, *depth)
	}

	query := fmt.Sprintf(`WITH RECURSIVE sub(id, lvl) AS (
    SELECT id, 0 FROM folders WHERE id = ?
    UNION ALL
    SELECT folders.id, sub.lvl + 1 FROM folders JOIN sub ON folders.parent_folder_id = sub.id %s
)
SELECT COUNT(DISTINCT images_files.image_id)
FROM images_files
JOIN files ON files.id = images_files.file_id
WHERE files.parent_folder_id IN (SELECT id FROM sub)`, levelClause)

	wrapper := dbWrapperType{}
	rows, err := wrapper.QueryxContext(ctx, query, args...)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("counting images in folder tree %d: %w", id, err)
	}
	defer rows.Close()

	var count int
	if rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	return count, nil
}

// TotalSizeInTree returns the total size in bytes of every file in the folder and, recursively, all
// its sub-folders. Recursion is always unlimited — a folder's size is its whole subtree.
func (qb *FolderStore) TotalSizeInTree(ctx context.Context, id models.FolderID) (int64, error) {
	const query = `WITH RECURSIVE sub(id) AS (
    SELECT id FROM folders WHERE id = ?
    UNION ALL
    SELECT folders.id FROM folders JOIN sub ON folders.parent_folder_id = sub.id
)
SELECT COALESCE(SUM(files.size), 0) FROM files WHERE files.parent_folder_id IN (SELECT id FROM sub)`

	wrapper := dbWrapperType{}
	rows, err := wrapper.QueryxContext(ctx, query, id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("summing file sizes in folder tree %d: %w", id, err)
	}
	defer rows.Close()

	var total int64
	if rows.Next() {
		if err := rows.Scan(&total); err != nil {
			return 0, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	return total, nil
}

// CountAllInPaths returns a count of all folders that are within any of the given paths.
// Returns count of all folders if p is empty.
func (qb *FolderStore) CountAllInPaths(ctx context.Context, p []string) (int, error) {
	q := qb.countDataset().Prepared(true)
	q = qb.allInPaths(q, p)

	return count(ctx, q)
}

// func (qb *FolderStore) findBySubquery(ctx context.Context, sq *goqu.SelectDataset) ([]*file.Folder, error) {
// 	table := qb.table()

// 	q := qb.selectDataset().Prepared(true).Where(
// 		table.Col(idColumn).Eq(
// 			sq,
// 		),
// 	)

// 	return qb.getMany(ctx, q)
// }

func (qb *FolderStore) FindByZipFileID(ctx context.Context, zipFileID models.FileID) ([]*models.Folder, error) {
	table := qb.table()

	q := qb.selectDataset().Prepared(true).Where(
		table.Col("zip_file_id").Eq(zipFileID),
	)

	return qb.getMany(ctx, q)
}

func (qb *FolderStore) validateFilter(fileFilter *models.FolderFilterType) error {
	const and = "AND"
	const or = "OR"
	const not = "NOT"

	if fileFilter.And != nil {
		if fileFilter.Or != nil {
			return illegalFilterCombination(and, or)
		}
		if fileFilter.Not != nil {
			return illegalFilterCombination(and, not)
		}

		return qb.validateFilter(fileFilter.And)
	}

	if fileFilter.Or != nil {
		if fileFilter.Not != nil {
			return illegalFilterCombination(or, not)
		}

		return qb.validateFilter(fileFilter.Or)
	}

	if fileFilter.Not != nil {
		return qb.validateFilter(fileFilter.Not)
	}

	return nil
}

func (qb *FolderStore) Query(ctx context.Context, options models.FolderQueryOptions) (*models.FolderQueryResult, error) {
	folderFilter := options.FolderFilter
	findFilter := options.FindFilter

	if folderFilter == nil {
		folderFilter = &models.FolderFilterType{}
	}
	if findFilter == nil {
		findFilter = &models.FindFilterType{}
	}

	query := qb.newQuery()

	distinctIDs(&query, folderTable)

	if q := findFilter.Q; q != nil && *q != "" {
		searchColumns := []string{"folders.path"}
		query.parseQueryString(searchColumns, *q)
	}

	if err := qb.validateFilter(folderFilter); err != nil {
		return nil, err
	}

	filter := filterBuilderFromHandler(ctx, &folderFilterHandler{
		folderFilter: folderFilter,
	})

	if err := query.addFilter(filter); err != nil {
		return nil, err
	}

	if err := qb.setQuerySort(&query, findFilter); err != nil {
		return nil, err
	}
	query.sortAndPagination += getPagination(findFilter)

	result, err := qb.queryGroupedFields(ctx, options, query)
	if err != nil {
		return nil, fmt.Errorf("error querying aggregate fields: %w", err)
	}

	idsResult, err := query.findIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("error finding IDs: %w", err)
	}

	result.IDs = make([]models.FolderID, len(idsResult))
	for i, id := range idsResult {
		result.IDs[i] = models.FolderID(id)
	}

	return result, nil
}

func (qb *FolderStore) queryGroupedFields(ctx context.Context, options models.FolderQueryOptions, query queryBuilder) (*models.FolderQueryResult, error) {
	if !options.Count {
		// nothing to do - return empty result
		return models.NewFolderQueryResult(qb), nil
	}

	aggregateQuery := qb.newQuery()

	if options.Count {
		aggregateQuery.addColumn("COUNT(DISTINCT temp.id) as total")
	}

	const includeSortPagination = false
	aggregateQuery.from = fmt.Sprintf("(%s) as temp", query.toSQL(includeSortPagination))

	out := struct {
		Total      int
		Duration   float64
		Megapixels float64
		Size       int64
	}{}
	if err := qb.repository.queryStruct(ctx, aggregateQuery.toSQL(includeSortPagination), query.allArgs(), &out); err != nil {
		return nil, err
	}

	ret := models.NewFolderQueryResult(qb)
	ret.Count = out.Total

	return ret, nil
}

var folderSortOptions = sortOptions{
	"created_at",
	"id",
	"path",
	"basename",
	"random",
	"updated_at",
}

func (qb *FolderStore) setQuerySort(query *queryBuilder, findFilter *models.FindFilterType) error {
	if findFilter == nil || findFilter.Sort == nil || *findFilter.Sort == "" {
		return nil
	}
	sort := findFilter.GetSort("path")

	// CVE-2024-32231 - ensure sort is in the list of allowed sorts
	if err := folderSortOptions.validateSort(sort); err != nil {
		return err
	}

	direction := findFilter.GetDirection()
	query.sortAndPagination += getSort(sort, direction, "folders")

	return nil
}
