package conversation

import (
	"time"

	"linq-bs/server/internal/agent/run"
	"linq-bs/server/internal/agent/runtime"
)

// contextObservations 将 Builder 计时与准备阶段补齐后的元数据对应，避免保存过早的分段副本。
func contextObservations(segments []runtime.Segment, measured []run.ContextObservation) []run.ContextObservation {
	result := make([]run.ContextObservation, 0, len(segments))
	used := make([]bool, len(measured))
	for _, segment := range segments {
		observation := run.ContextObservation{Slot: segment.Name, Segment: segment, StartedAt: time.Now(), EndedAt: time.Now()}
		for i, candidate := range measured {
			if !used[i] && candidate.Segment.Name == segment.Name {
				observation.Slot, observation.StartedAt, observation.EndedAt = candidate.Slot, candidate.StartedAt, candidate.EndedAt
				used[i] = true
				break
			}
		}
		result = append(result, observation)
	}
	return result
}

func observeContext(observations *[]run.ContextObservation) func(string, runtime.Segment, time.Duration) {
	return func(slot string, segment runtime.Segment, elapsed time.Duration) {
		ended := time.Now()
		*observations = append(*observations, run.ContextObservation{Slot: slot, Segment: segment, StartedAt: ended.Add(-elapsed), EndedAt: ended})
	}
}
