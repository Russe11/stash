package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
)

type viewDateManager struct {
	tableMgr *viewHistoryTable
}

func (qb *viewDateManager) GetViewDates(ctx context.Context, id int) ([]time.Time, error) {
	return qb.tableMgr.getDates(ctx, id)
}

func (qb *viewDateManager) GetManyViewDates(ctx context.Context, ids []int) ([][]time.Time, error) {
	return qb.tableMgr.getManyDates(ctx, ids)
}

func (qb *viewDateManager) CountViews(ctx context.Context, id int) (int, error) {
	historyCount, err := qb.tableMgr.getCount(ctx, id)
	if err != nil {
		return 0, err
	}

	aggregateCount, err := countAggregatePlays(ctx, id)
	if err != nil {
		return 0, err
	}

	return historyCount + aggregateCount, nil
}

func (qb *viewDateManager) GetManyViewCount(ctx context.Context, ids []int) ([]int, error) {
	historyCounts, err := qb.tableMgr.getManyCount(ctx, ids)
	if err != nil {
		return nil, err
	}

	aggregateCounts, err := countManyAggregatePlays(ctx, ids)
	if err != nil {
		return nil, err
	}

	for i := range historyCounts {
		historyCounts[i] += aggregateCounts[i]
	}

	return historyCounts, nil
}

func (qb *viewDateManager) CountAllViews(ctx context.Context) (int, error) {
	historyCount, err := qb.tableMgr.getAllCount(ctx)
	if err != nil {
		return 0, err
	}

	aggregateCount, err := countAllAggregatePlays(ctx)
	if err != nil {
		return 0, err
	}

	return historyCount + aggregateCount, nil
}

func (qb *viewDateManager) CountUniqueViews(ctx context.Context) (int, error) {
	query := fmt.Sprintf(
		"SELECT COUNT(*) FROM (SELECT %s FROM %s GROUP BY %s UNION SELECT %s FROM %s WHERE %s > 0)",
		sceneIDColumn,
		scenesViewDatesTable,
		sceneIDColumn,
		sceneIDColumn,
		scenesPlayCountsTable,
		scenePlayCountColumn,
	)

	var ret int
	if err := dbWrapper.Get(ctx, &ret, query); err != nil {
		return 0, err
	}

	return ret, nil
}

func (qb *viewDateManager) LastView(ctx context.Context, id int) (*time.Time, error) {
	return qb.tableMgr.getLastDate(ctx, id)
}

func (qb *viewDateManager) GetManyLastViewed(ctx context.Context, ids []int) ([]*time.Time, error) {
	return qb.tableMgr.getManyLastDate(ctx, ids)

}

func (qb *viewDateManager) AddViews(ctx context.Context, id int, dates []time.Time) ([]time.Time, error) {
	return qb.tableMgr.addDates(ctx, id, dates)
}

func (qb *viewDateManager) IncrementPlayCount(ctx context.Context, id int) (int, error) {
	query := fmt.Sprintf(
		"INSERT INTO %s (%s, %s) VALUES (?, 1) ON CONFLICT(%s) DO UPDATE SET %s = %s + 1",
		scenesPlayCountsTable,
		sceneIDColumn,
		scenePlayCountColumn,
		sceneIDColumn,
		scenePlayCountColumn,
		scenePlayCountColumn,
	)
	if _, err := dbWrapper.Exec(ctx, query, id); err != nil {
		return 0, fmt.Errorf("incrementing aggregate play count for scene %d: %w", id, err)
	}

	return qb.CountViews(ctx, id)
}

func (qb *viewDateManager) DeleteViews(ctx context.Context, id int, dates []time.Time) ([]time.Time, error) {
	return qb.tableMgr.deleteDates(ctx, id, dates)
}

func (qb *viewDateManager) DeleteAllViews(ctx context.Context, id int) (int, error) {
	if _, err := qb.tableMgr.deleteAllDates(ctx, id); err != nil {
		return 0, err
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE %s = ?", scenesPlayCountsTable, sceneIDColumn)
	if _, err := dbWrapper.Exec(ctx, query, id); err != nil {
		return 0, fmt.Errorf("resetting aggregate play count for scene %d: %w", id, err)
	}

	return qb.CountViews(ctx, id)
}

func countAggregatePlays(ctx context.Context, id int) (int, error) {
	table := goqu.T(scenesPlayCountsTable)
	q := dialect.Select(goqu.COALESCE(table.Col(scenePlayCountColumn), 0)).From(table).Where(table.Col(sceneIDColumn).Eq(id))

	var ret int
	if err := queryFunc(ctx, q, true, func(rows *sqlx.Rows) error {
		return rows.Scan(&ret)
	}); err != nil {
		return 0, err
	}

	return ret, nil
}

func countManyAggregatePlays(ctx context.Context, ids []int) ([]int, error) {
	ret := make([]int, len(ids))
	if len(ids) == 0 {
		return ret, nil
	}

	table := goqu.T(scenesPlayCountsTable)
	q := dialect.Select(table.Col(sceneIDColumn), table.Col(scenePlayCountColumn)).From(table).Where(table.Col(sceneIDColumn).In(ids))
	idToIndex := idToIndexMap(ids)

	if err := queryFunc(ctx, q, false, func(rows *sqlx.Rows) error {
		var id int
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return err
		}
		ret[idToIndex[id]] = count
		return nil
	}); err != nil {
		return nil, err
	}

	return ret, nil
}

func countAllAggregatePlays(ctx context.Context) (int, error) {
	table := goqu.T(scenesPlayCountsTable)
	q := dialect.Select(goqu.COALESCE(goqu.SUM(table.Col(scenePlayCountColumn)), 0)).From(table)

	var ret int
	if err := queryFunc(ctx, q, true, func(rows *sqlx.Rows) error {
		return rows.Scan(&ret)
	}); err != nil {
		return 0, err
	}

	return ret, nil
}

type oDateManager struct {
	tableMgr *viewHistoryTable
}

func (qb *oDateManager) GetODates(ctx context.Context, id int) ([]time.Time, error) {
	return qb.tableMgr.getDates(ctx, id)
}

func (qb *oDateManager) GetManyODates(ctx context.Context, ids []int) ([][]time.Time, error) {
	return qb.tableMgr.getManyDates(ctx, ids)
}

func (qb *oDateManager) GetOCount(ctx context.Context, id int) (int, error) {
	return qb.tableMgr.getCount(ctx, id)
}

func (qb *oDateManager) GetManyOCount(ctx context.Context, ids []int) ([]int, error) {
	return qb.tableMgr.getManyCount(ctx, ids)
}

func (qb *oDateManager) GetAllOCount(ctx context.Context) (int, error) {
	return qb.tableMgr.getAllCount(ctx)
}

func (qb *oDateManager) GetUniqueOCount(ctx context.Context) (int, error) {
	return qb.tableMgr.getUniqueCount(ctx)
}

func (qb *oDateManager) AddO(ctx context.Context, id int, dates []time.Time) ([]time.Time, error) {
	return qb.tableMgr.addDates(ctx, id, dates)
}

func (qb *oDateManager) DeleteO(ctx context.Context, id int, dates []time.Time) ([]time.Time, error) {
	return qb.tableMgr.deleteDates(ctx, id, dates)
}

func (qb *oDateManager) ResetO(ctx context.Context, id int) (int, error) {
	return qb.tableMgr.deleteAllDates(ctx, id)
}
