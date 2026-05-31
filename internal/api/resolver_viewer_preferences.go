package api

import (
	"context"

	"github.com/stashapp/stash/internal/manager/config"
)

func (r *queryResolver) ViewerPreferences(ctx context.Context) (*ViewerPreferences, error) {
	return makeViewerPreferencesResult(), nil
}

func (r *mutationResolver) UpdateViewerPreferences(ctx context.Context, input ViewerPreferencesInput) (*ViewerPreferences, error) {
	c := config.GetInstance()

	if input.StashTv != nil && input.StashTv.Home != nil && input.StashTv.Home.ExcludedTagIDs != nil {
		c.SetStashTVHomeExcludedTagIDs(input.StashTv.Home.ExcludedTagIDs)
	}

	if err := c.Write(); err != nil {
		return makeViewerPreferencesResult(), err
	}

	return makeViewerPreferencesResult(), nil
}

func makeViewerPreferencesResult() *ViewerPreferences {
	c := config.GetInstance()
	return &ViewerPreferences{
		StashTv: &StashTVViewerPreferences{
			Home: &StashTVHomePreferences{
				ExcludedTagIDs: c.GetStashTVHomeExcludedTagIDs(),
			},
		},
	}
}
