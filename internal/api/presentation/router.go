// Package apirouter provides API router layer functionalities.
package apirouter

import (
	"github.com/codaxa/tunnelR.git/internal/api/app/service"
	"github.com/codaxa/tunnelR.git/internal/api/presentation/handlers"
	"github.com/codaxa/tunnelR.git/internal/api/presentation/middleware"
	"github.com/go-chi/chi"
)

// NewRouter creates a new HTTP router with the necessary routes and middleware.
func NewRouter(authService *service.AuthService, teamService *service.TeamService, machineService *service.MachineService) *chi.Mux {
	router := chi.NewRouter()

	userHandler := handlers.NewUserHandler(authService)
	teamHandler := handlers.NewTeamHandler(teamService)
	machineHandler := handlers.NewMachineHandler(machineService)
	authMiddleware := middleware.NewAuthMiddleware(authService)

	// Public routes
	router.Get("/healthz", handlers.HealthHandler)
	router.Get("/version", handlers.VersionHandler)

	// API routes
	router.Route("/api/v1", func(r chi.Router) {
		// Public API endpoints
		r.Post("/login", userHandler.Login)

		// Authenticated user routes
		r.Group(func(r chi.Router) {
			r.Use(authMiddleware.Authenticate)
			r.Get("/whoami", userHandler.GetUserInfo)

			r.Get("/teams", teamHandler.GetTeams)
			r.Get("/user/machines", userHandler.GetUserMachines)
			r.Get("/machines/id/{id}", machineHandler.GetMachineByID)
			r.Get("/machines/ip/{ip}", machineHandler.GetMachineByIPAddress)
		})

		// Admin-only routes
		r.Group(func(r chi.Router) {
			r.Use(authMiddleware.AdminAuthenticate)
			r.Post("/register", userHandler.Register)

			r.Post("/teams", teamHandler.CreateTeam)
			r.Delete("/teams/{id}", teamHandler.DeleteTeam)
			r.Get("/teams/{id}", teamHandler.GetTeam)
			r.Get("/teams/{id}/machines", teamHandler.GetTeamMachines)

			r.Post("/machines", machineHandler.CreateMachine)
			r.Put("/machines", machineHandler.UpdateMachine)
			r.Delete("/machines/{id}", machineHandler.DeleteMachine)
			r.Get("/machines/{id}/teams", machineHandler.GetTeamsForMachine)
			r.Post("/machines/teams", machineHandler.AddTeamToMachine)
			r.Delete("/machines/teams", machineHandler.RemoveTeamFromMachine)
		})
	})

	return router
}
