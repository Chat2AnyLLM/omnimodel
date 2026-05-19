package routes

import (
	"github.com/gin-gonic/gin"
)

func SetupAdminRoutes(router *gin.RouterGroup, port int) {
	// Provider management
	router.GET("/providers", handleGetProviders)
	router.POST("/providers/switch", handleSwitchProvider)
	router.GET("/providers/priorities", handleGetProviderPriorities)
	router.POST("/providers/priorities", handleSetProviderPriorities)

	// Instance-specific routes (all :id routes before :type routes)
	router.DELETE("/providers/:id", handleDeleteProvider)
	router.GET("/providers/:id/models", handleListProviderModels)
	router.POST("/providers/:id/models/refresh", handleRefreshProviderModels)
	router.POST("/providers/:id/models/toggle", handleToggleProviderModel)
	router.GET("/providers/:id/models/:modelId/version", handleGetModelVersion)
	router.PUT("/providers/:id/models/:modelId/version", handleSetModelVersion)
	router.GET("/providers/:id/usage", handleGetProviderUsage)
	router.POST("/providers/:id/auth", handleProviderAuth)
	router.POST("/providers/:id/auth/initiate-device-code", handleInitiateDeviceCode)
	router.POST("/providers/:id/auth/complete-device-code", handleCompleteDeviceCode)
	router.PUT("/providers/:id/config", handleUpdateProviderConfig)
	router.PATCH("/providers/:id/name", handleRenameProvider)
	router.POST("/providers/:id/activate", handleActivateProvider)
	router.POST("/providers/:id/deactivate", handleDeactivateProvider)

	// Provider type-specific routes (use specific path to avoid conflicts with wildcard :id routes)
	router.POST("/providers/auth-and-create/:type", handleAuthAndCreateProvider)

	// Antigravity Google OAuth2 authorization-code flow
	// Note: oauth-callback and oauth-status are registered on the public group in server.go
	router.POST("/providers/antigravity/start-oauth", handleAntigravityStartOAuth)

	// System info and status
	router.GET("/status", handleGetStatus)
	router.GET("/auth-status", handleGetAuthStatus)
	router.POST("/auth/cancel", handleCancelAuth)

	// Settings
	router.GET("/settings/log-level", handleGetLogLevel)
	router.PUT("/settings/log-level", handleSetLogLevel)
	router.POST("/settings/test-log", handleTestLog)
	router.POST("/settings/debug-log", handleDebugLog)

	// Chat sessions
	router.GET("/chat/sessions", handleGetChatSessions)
	router.POST("/chat/sessions", handleCreateChatSession)
	router.DELETE("/chat/sessions", handleDeleteAllChatSessions)
	router.GET("/chat/sessions/:id", handleGetChatSession)
	router.PUT("/chat/sessions/:id", handleUpdateChatSession)
	router.POST("/chat/sessions/:id/messages", handleAddChatMessage)
	router.DELETE("/chat/sessions/:id", handleDeleteChatSession)

	// Logs streaming
	router.GET("/logs/stream", handleLogsStream)

	// Metering / usage data
	SetupMeteringRoutes(router)

	// Access token management
	SetupAccessTokenRoutes(router)

	// Config file management
	router.GET("/config", handleGetConfigFiles)
	router.GET("/config/:name", handleGetConfig)
	router.PUT("/config/:name", handleSaveConfig)
	router.POST("/config/:name/import", handleImportConfig)
	router.POST("/config/:name/backup", handleBackupConfig)
}
