package api

import (
	"context"
	"time"

	"github.com/omi-teams/omi-board/server/internal/health"
)

type Server struct{ DB health.Pinger }

var _ StrictServerInterface = Server{}

func (s Server) GetV1Healthz(ctx context.Context, _ GetV1HealthzRequestObject) (GetV1HealthzResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := s.DB.Ping(ctx); err != nil {
		return GetV1Healthz503ApplicationProblemPlusJSONResponse(Problem{
			Type: "about:blank", Title: "Database unavailable", Status: 503, Code: "database_unavailable",
		}), nil
	}
	return GetV1Healthz200JSONResponse(Health{Status: "ok"}), nil
}
