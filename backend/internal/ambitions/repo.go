package ambitions

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"lexis/internal/ambitions/store"
	"lexis/internal/platform/clock"
)

func dateString(d *time.Time) *string {
	if d == nil {
		return nil
	}
	v := clock.FormatDate(*d)
	return &v
}
func dateValue(d *string) *time.Time {
	if d == nil {
		return nil
	}
	v, _ := clock.ParseDate(*d)
	return &v
}

// Read uses the caller's transaction, including the full backup's repeatable-read snapshot.
func Read(ctx context.Context, db store.DBTX) (Snapshot, error) {
	q := store.New(db)
	out := Empty()
	rowsCategory, err := q.ListCategories(ctx)
	if err != nil {
		return out, fmt.Errorf("read Categories: %w", err)
	}
	for _, r := range rowsCategory {
		out.Categories = append(out.Categories, Category{ID: r.ID, Title: r.Title, Position: r.Position})
	}
	rowsProject, err := q.ListProjects(ctx)
	if err != nil {
		return out, fmt.Errorf("read Projects: %w", err)
	}
	for _, r := range rowsProject {
		out.Projects = append(out.Projects, Project{ID: r.ID, Fields: Fields{Title: r.Title, Note: r.Note, Deadline: dateString(r.Deadline), Position: r.Position}})
	}
	rowsDream, err := q.ListDreams(ctx)
	if err != nil {
		return out, fmt.Errorf("read Dreams: %w", err)
	}
	for _, r := range rowsDream {
		out.Dreams = append(out.Dreams, Dream{ID: r.ID, Fields: Fields{Title: r.Title, Note: r.Note, Deadline: dateString(r.Deadline), Position: r.Position}, Status: r.Status, CategoryID: r.CategoryID, ProjectID: r.ProjectID})
	}
	rowsStep, err := q.ListSteps(ctx)
	if err != nil {
		return out, fmt.Errorf("read Steps: %w", err)
	}
	for _, r := range rowsStep {
		out.Steps = append(out.Steps, Step{ID: r.ID, Fields: Fields{Title: r.Title, Note: r.Note, Deadline: dateString(r.Deadline), Position: r.Position}, ProjectID: r.ProjectID, ParentID: r.ParentID, Done: r.Done})
	}
	ds, err := q.ListDependencies(ctx)
	if err != nil {
		return out, fmt.Errorf("read dependencies: %w", err)
	}
	for _, d := range ds {
		out.Dependencies = append(out.Dependencies, Dependency{StepID: d.StepID, RequiresID: d.RequiresID})
	}
	if err := readTimeline(ctx, q, &out); err != nil {
		return out, err
	}
	return out, nil
}

// Write updates changed rows only; all foreign keys are deferred until the transaction commits.
// A nil before means restore: every row is written and rows absent from the file are removed.
func Write(ctx context.Context, db store.DBTX, before *Snapshot, after Snapshot) error {
	q := store.New(db)
	oldCategory := map[int64]Category{}
	if before != nil {
		for _, v := range before.Categories {
			oldCategory[v.ID] = v
		}
	}
	idsCategory := []int64{}
	for _, v := range after.Categories {
		idsCategory = append(idsCategory, v.ID)
		if prev, ok := oldCategory[v.ID]; ok && reflect.DeepEqual(prev, v) {
			continue
		}
		if err := q.SaveCategory(ctx, store.SaveCategoryParams{ID: v.ID, Title: v.Title, Position: v.Position}); err != nil {
			return fmt.Errorf("save Category: %w", err)
		}
	}
	oldProject := map[int64]Project{}
	if before != nil {
		for _, v := range before.Projects {
			oldProject[v.ID] = v
		}
	}
	idsProject := []int64{}
	for _, v := range after.Projects {
		idsProject = append(idsProject, v.ID)
		if prev, ok := oldProject[v.ID]; ok && reflect.DeepEqual(prev, v) {
			continue
		}
		if err := q.SaveProject(ctx, store.SaveProjectParams{ID: v.ID, Title: v.Title, Position: v.Position, Note: v.Note, Deadline: dateValue(v.Deadline)}); err != nil {
			return fmt.Errorf("save Project: %w", err)
		}
	}
	oldDream := map[int64]Dream{}
	if before != nil {
		for _, v := range before.Dreams {
			oldDream[v.ID] = v
		}
	}
	idsDream := []int64{}
	for _, v := range after.Dreams {
		idsDream = append(idsDream, v.ID)
		if prev, ok := oldDream[v.ID]; ok && reflect.DeepEqual(prev, v) {
			continue
		}
		if err := q.SaveDream(ctx, store.SaveDreamParams{ID: v.ID, Title: v.Title, Position: v.Position, Note: v.Note, Deadline: dateValue(v.Deadline), Status: v.Status, CategoryID: v.CategoryID, ProjectID: v.ProjectID}); err != nil {
			return fmt.Errorf("save Dream: %w", err)
		}
	}
	oldStep := map[int64]Step{}
	if before != nil {
		for _, v := range before.Steps {
			oldStep[v.ID] = v
		}
	}
	idsStep := []int64{}
	for _, v := range after.Steps {
		idsStep = append(idsStep, v.ID)
		if prev, ok := oldStep[v.ID]; ok && reflect.DeepEqual(prev, v) {
			continue
		}
		if err := q.SaveStep(ctx, store.SaveStepParams{ID: v.ID, Title: v.Title, Position: v.Position, Note: v.Note, Deadline: dateValue(v.Deadline), ProjectID: v.ProjectID, ParentID: v.ParentID, Done: v.Done}); err != nil {
			return fmt.Errorf("save Step: %w", err)
		}
	}
	if before == nil || !reflect.DeepEqual(before.Dependencies, after.Dependencies) {
		if err := q.ClearDependencies(ctx); err != nil {
			return err
		}
		projects := map[int64]int64{}
		for _, n := range after.Steps {
			projects[n.ID] = n.ProjectID
		}
		for _, d := range after.Dependencies {
			if err := q.SaveDependency(ctx, store.SaveDependencyParams{ProjectID: projects[d.StepID], StepID: d.StepID, RequiresID: d.RequiresID}); err != nil {
				return err
			}
		}
	}
	if err := q.PruneSteps(ctx, idsStep); err != nil {
		return fmt.Errorf("prune Steps: %w", err)
	}
	if err := writeTimeline(ctx, q, after.Timeline); err != nil {
		return err
	}
	if err := q.PruneDreams(ctx, idsDream); err != nil {
		return fmt.Errorf("prune Dreams: %w", err)
	}
	if err := q.PruneProjects(ctx, idsProject); err != nil {
		return fmt.Errorf("prune Projects: %w", err)
	}
	if err := q.PruneCategories(ctx, idsCategory); err != nil {
		return fmt.Errorf("prune Categories: %w", err)
	}
	return nil
}
