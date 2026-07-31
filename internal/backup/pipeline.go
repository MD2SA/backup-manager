package backup

import (
	"fmt"
	"time"
)

type Pipeline struct {
	Stages []Stage
}

func NewPipeline(stages ...Stage) *Pipeline {
	return &Pipeline{Stages: stages}
}

func (p *Pipeline) Run(ctx *ExecutionContext) error {
	ctx.StartTime = time.Now()
	ctx.Log("Starting backup pipeline")

	var runErr error
	for _, stage := range p.Stages {
		if stage.Name() == "Cleanup" {
			continue // Handle cleanup at the end
		}

		ctx.Log(fmt.Sprintf("Entering stage: %s", stage.Name()))
		if err := stage.Execute(ctx); err != nil {
			ctx.Error = err
			ctx.Log(fmt.Sprintf("Stage %s failed: %v", stage.Name(), err))
			runErr = err
			break
		}
		ctx.Log(fmt.Sprintf("Stage %s completed successfully", stage.Name()))
	}

	// Always attempt cleanup if stage exists
	for _, stage := range p.Stages {
		if stage.Name() == "Cleanup" {
			ctx.Log("Entering cleanup stage")
			if err := stage.Execute(ctx); err != nil {
				ctx.Log(fmt.Sprintf("Cleanup failed: %v", err))
			}
		}
	}

	ctx.EndTime = time.Now()
	if runErr == nil {
		ctx.Log("Backup pipeline completed successfully")
	}
	return runErr
}
