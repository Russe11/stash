package api

import (
	"context"
	"slices"
	"strconv"

	"github.com/99designs/gqlgen/graphql"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/scene"
)

func (r *queryResolver) FindScene(ctx context.Context, id *string, checksum *string) (*models.Scene, error) {
	var scene *models.Scene
	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		qb := r.repository.Scene
		var err error
		if id != nil {
			idInt, err := strconv.Atoi(*id)
			if err != nil {
				return err
			}
			scene, err = qb.Find(ctx, idInt)
			if err != nil {
				return err
			}
		} else if checksum != nil {
			var scenes []*models.Scene
			scenes, err = qb.FindByChecksum(ctx, *checksum)
			if len(scenes) > 0 {
				scene = scenes[0]
			}
		}

		return err
	}); err != nil {
		return nil, err
	}

	return scene, nil
}

func (r *queryResolver) FindSceneByHash(ctx context.Context, input SceneHashInput) (*models.Scene, error) {
	var scene *models.Scene

	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		qb := r.repository.Scene
		if input.Checksum != nil {
			scenes, err := qb.FindByChecksum(ctx, *input.Checksum)
			if err != nil {
				return err
			}
			if len(scenes) > 0 {
				scene = scenes[0]
			}
		}

		if scene == nil && input.Oshash != nil {
			scenes, err := qb.FindByOSHash(ctx, *input.Oshash)
			if err != nil {
				return err
			}
			if len(scenes) > 0 {
				scene = scenes[0]
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return scene, nil
}

func (r *queryResolver) FindScenes(
	ctx context.Context,
	sceneFilter *models.SceneFilterType,
	sceneIDs []int,
	ids []string,
	filter *models.FindFilterType,
) (ret *FindScenesResultType, err error) {
	if len(ids) > 0 {
		sceneIDs, err = handleIDList(ids, "ids")
		if err != nil {
			return nil, err
		}
	}

	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		var scenes []*models.Scene
		var err error

		fields := graphql.CollectAllFields(ctx)
		result := &models.SceneQueryResult{}

		if len(sceneIDs) > 0 {
			scenes, err = r.repository.Scene.FindMany(ctx, sceneIDs)
			if err == nil {
				result.Count = len(scenes)
				for _, s := range scenes {
					if err = s.LoadPrimaryFile(ctx, r.repository.File); err != nil {
						break
					}

					f := s.Files.Primary()
					if f == nil {
						continue
					}

					result.TotalDuration += f.Duration

					result.TotalSize += float64(f.Size)
				}
			}
		} else {
			result, err = r.repository.Scene.Query(ctx, models.SceneQueryOptions{
				QueryOptions: models.QueryOptions{
					FindFilter: filter,
					Count:      slices.Contains(fields, "count"),
				},
				SceneFilter:   sceneFilter,
				TotalDuration: slices.Contains(fields, "duration"),
				TotalSize:     slices.Contains(fields, "filesize"),
			})
			if err == nil {
				scenes, err = result.Resolve(ctx)
			}
		}

		if err != nil {
			return err
		}

		ret = &FindScenesResultType{
			Count:    result.Count,
			Scenes:   scenes,
			Duration: result.TotalDuration,
			Filesize: result.TotalSize,
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return ret, nil
}

func (r *queryResolver) FindScenesByPathRegex(ctx context.Context, filter *models.FindFilterType) (ret *FindScenesResultType, err error) {
	if err := r.withReadTxn(ctx, func(ctx context.Context) error {

		sceneFilter := &models.SceneFilterType{}

		if filter != nil && filter.Q != nil {
			sceneFilter.Path = &models.StringCriterionInput{
				Modifier: models.CriterionModifierMatchesRegex,
				Value:    "(?i)" + *filter.Q,
			}
		}

		// make a copy of the filter if provided, nilling out Q
		var queryFilter *models.FindFilterType
		if filter != nil {
			f := *filter
			queryFilter = &f
			queryFilter.Q = nil
		}

		fields := graphql.CollectAllFields(ctx)

		result, err := r.repository.Scene.Query(ctx, models.SceneQueryOptions{
			QueryOptions: models.QueryOptions{
				FindFilter: queryFilter,
				Count:      slices.Contains(fields, "count"),
			},
			SceneFilter:   sceneFilter,
			TotalDuration: slices.Contains(fields, "duration"),
			TotalSize:     slices.Contains(fields, "filesize"),
		})
		if err != nil {
			return err
		}

		scenes, err := result.Resolve(ctx)
		if err != nil {
			return err
		}

		ret = &FindScenesResultType{
			Count:    result.Count,
			Scenes:   scenes,
			Duration: result.TotalDuration,
			Filesize: result.TotalSize,
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return ret, nil
}

func (r *queryResolver) ParseSceneFilenames(ctx context.Context, filter *models.FindFilterType, config models.SceneParserInput) (ret *SceneParserResultType, err error) {
	repo := scene.NewFilenameParserRepository(r.repository)
	parser := scene.NewFilenameParser(filter, config, repo)

	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		result, count, err := parser.Parse(ctx)

		if err != nil {
			return err
		}

		ret = &SceneParserResultType{
			Count:   count,
			Results: result,
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return ret, nil
}

func (r *queryResolver) FindDuplicateScenes(ctx context.Context, distance *int, durationDiff *float64) (ret [][]*models.Scene, err error) {
	dist := 0
	durDiff := -1.
	if distance != nil {
		dist = *distance
	}
	if durationDiff != nil {
		durDiff = *durationDiff
	}
	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		ret, err = r.repository.Scene.FindDuplicates(ctx, dist, durDiff)
		return err
	}); err != nil {
		return nil, err
	}

	return ret, nil
}

// Upper bounds on findSimilarScenes inputs. A phash is a 64-bit value, so a
// Hamming distance above 64 matches every scene; and an unbounded limit flows
// straight into the SQL LIMIT, then into an in-memory FindMany of every matched
// scene — a cheap way for one query to OOM the server. Clamp both at the API
// boundary. The schema defaults (distance 10, limit 40) sit far below these, so
// real "more like this" callers are unaffected.
const (
	maxSimilarDistance = 64
	maxSimilarLimit    = 1000
)

func clampSimilarDistance(d int) int {
	switch {
	case d < 0:
		return 0
	case d > maxSimilarDistance:
		return maxSimilarDistance
	default:
		return d
	}
}

func clampSimilarLimit(l int) int {
	switch {
	case l < 0:
		return 0
	case l > maxSimilarLimit:
		return maxSimilarLimit
	default:
		return l
	}
}

func (r *queryResolver) FindSimilarScenes(ctx context.Context, sceneID string, distance *int, limit *int) (ret []*SimilarSceneResult, err error) {
	id, err := strconv.Atoi(sceneID)
	if err != nil {
		return nil, err
	}

	// Defaults match the schema (distance: 10, limit: 40) for direct/non-GraphQL
	// callers; gqlgen also supplies these via the SDL default values.
	dist := 10
	if distance != nil {
		dist = *distance
	}
	lim := 40
	if limit != nil {
		lim = *limit
	}
	// Bound caller-supplied values so a hostile/buggy request can't request the
	// whole library (see maxSimilar* above).
	dist = clampSimilarDistance(dist)
	lim = clampSimilarLimit(lim)

	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		similar, err := r.repository.Scene.FindSimilar(ctx, id, dist, lim)
		if err != nil {
			return err
		}
		if len(similar) == 0 {
			ret = []*SimilarSceneResult{}
			return nil
		}

		ids := make([]int, len(similar))
		for i, s := range similar {
			ids[i] = s.ID
		}

		// FindMany returns scenes in the same order as ids, so the nearest-first
		// ordering from FindSimilar is preserved.
		scenes, err := r.repository.Scene.FindMany(ctx, ids)
		if err != nil {
			return err
		}

		ret = make([]*SimilarSceneResult, len(similar))
		for i, s := range similar {
			ret[i] = &SimilarSceneResult{
				Scene:    scenes[i],
				Distance: s.Distance,
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return ret, nil
}

func (r *queryResolver) AllScenes(ctx context.Context) (ret []*models.Scene, err error) {
	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		ret, err = r.repository.Scene.All(ctx)
		return err
	}); err != nil {
		return nil, err
	}

	return capAllResults("Scenes", ret), nil
}
