package server

import "context"

func (s *Server) markRoadmapQuestionPending(ctx context.Context, sessionID, requestID string) {
	if s.core == nil || requestID == "" || sessionID == "" {
		return
	}
	moved, err := s.core.MoveRoadmapSessionTaskForQuestion(ctx, sessionID)
	if err != nil {
		return
	}
	s.input.attach(requestID, sessionID, moved)
}

func (s *Server) markRoadmapQuestionResolved(ctx context.Context, requestID string) bool {
	if s.core == nil || requestID == "" {
		return false
	}
	meta := s.input.popMeta(requestID)
	if !meta.restoreRoadmap || meta.sessionID == "" {
		return false
	}
	_ = s.core.RestoreRoadmapSessionTaskAfterQuestion(ctx, meta.sessionID)
	return true
}
