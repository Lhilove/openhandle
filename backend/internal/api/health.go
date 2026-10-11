package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Healthz is a lightweight liveness probe.
func (s *Server) Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Readyz verifies dependencies (database, and RPC reachability informationally).
func (s *Server) Readyz(c *gin.Context) {
	ctx := c.Request.Context()
	dbOK := s.Store.Pool.Ping(ctx) == nil
	rpcOK := s.RPC.Healthy(ctx)

	status := http.StatusOK
	if !dbOK {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{
		"status":  map[bool]string{true: "ready", false: "degraded"}[dbOK],
		"db":      dbOK,
		"rpc":     rpcOK,
		"cluster": s.Cfg.SolanaCluster,
	})
}
