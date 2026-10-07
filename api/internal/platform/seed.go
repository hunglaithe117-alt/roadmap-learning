package platform

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	contentapp "langapp/internal/application/content"
	contentinfra "langapp/internal/infrastructure/content"
)

// seedStep represents a bootstrap seeding step.
type seedStep struct {
	name string
	run  func(context.Context) error
}

// Seed runs all bootstrap seeders idempotently.
func (c *Container) Seed(ctx context.Context) error {
	if c == nil || c.RoadmapSeeder == nil || c.Content == nil {
		return fmt.Errorf("platform: uninitialized seeders (roadmap=%v content=%v)",
			c != nil && c.RoadmapSeeder != nil, c != nil && c.Content != nil)
	}

	steps := c.seedSteps()
	failed := make([]string, 0, len(steps))
	for _, s := range steps {
		err := c.BootStep(ctx, "seed "+s.name, s.run)
		if err == nil {
			continue
		}
		failed = append(failed, s.name)
		c.Logger.Error("seeder failed, continuing boot",
			slog.String("seeder", s.name), slog.String("err", err.Error()))
	}
	if len(failed) > 0 {
		return fmt.Errorf("platform: %d/%d seeders failed: %s",
			len(failed), len(steps), strings.Join(failed, ", "))
	}
	return nil
}

// seedSteps lists bootstrap seeders in execution order.
func (c *Container) seedSteps() []seedStep {
	steps := []seedStep{{
		name: "roadmap",
		run:  func(ctx context.Context) error { return c.RoadmapSeeder.Run(ctx) },
	}}

	for _, level := range contentinfra.HskLevels {
		lvl := level
		steps = append(steps, seedStep{
			name: "hsk/" + lvl,
			run: func(ctx context.Context) error {
				res, err := c.Content.ImportHSK(ctx, contentapp.ImportInput{Level: lvl})
				if err != nil {
					return err
				}
				c.Logger.Info("seed complete",
					slog.String("seeder", "hsk/"+lvl),
					slog.Int("cards_added", res.CardsAdded),
					slog.Int("cards_total", res.CardsTotal),
					slog.Int("dict_added", res.DictAdded))
				return nil
			},
		})
	}

	steps = append(steps, seedStep{
		name: "english",
		run: func(ctx context.Context) error {
			res, err := c.Content.SeedEnglish(ctx)
			if err != nil {
				return err
			}
			c.Logger.Info("seed complete",
				slog.String("seeder", "english"),
				slog.Int("en_dict", res.EnDict),
				slog.Int("pvo_added", res.PVOAdded),
				slog.Int("tmrnd_added", res.TMRNDAdded))
			return nil
		},
	})
	return steps
}
