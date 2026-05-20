package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"omnillm/internal/database"
	"omnillm/internal/lib/ratelimit"
	alibabapkg "omnillm/internal/providers/alibaba"
	antigravitypkg "omnillm/internal/providers/antigravity"
	azurepkg "omnillm/internal/providers/azure"
	codexpkg "omnillm/internal/providers/codex"
	"omnillm/internal/providers/copilot"
	googlepkg "omnillm/internal/providers/google"
	kimipkg "omnillm/internal/providers/kimi"
	modelscopepkg "omnillm/internal/providers/modelscope"
	openaicompatprovider "omnillm/internal/providers/openaicompatprovider"
	"omnillm/internal/providers/types"
	"omnillm/internal/registry"
	"omnillm/internal/routes"
)

type StartOptions struct {
	Port                int
	Host                string
	Verbose             bool
	AccountType         string
	Manual              bool
	RateLimit           *int
	RateLimitWait       bool
	GithubToken         string
	ClaudeCode          bool
	Console             bool
	ShowToken           bool
	ProxyEnv            bool
	Provider            string
	APIKey              string
	AllowLocalEndpoints bool
	EnableConfigEdit    bool
}

func RunServer(options StartOptions) error {
	// Setup logging
	setupLogging(options.Verbose)

	// Initialize database
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	configDir := filepath.Join(homeDir, ".config", "omnillm")

	if err := database.InitializeDatabase(configDir); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	apiKey, err := resolveAPIKey(configDir, options.APIKey)
	if err != nil {
		return fmt.Errorf("failed to resolve api key: %w", err)
	}
	options.APIKey = apiKey
	routes.ConfigureSecurityOptions(routes.SecurityOptions{
		ShowToken:        options.ShowToken,
		EnableConfigEdit: options.EnableConfigEdit,
	})

	// Initialize provider registry
	providerRegistry := registry.GetProviderRegistry()

	// Register default providers
	if err := registerDefaultProviders(providerRegistry, options); err != nil {
		log.Warn().Err(err).Msg("Failed to register some providers")
	}

	// Configure rate limiter
	rateLimitInterval := 0
	if options.RateLimit != nil {
		rateLimitInterval = *options.RateLimit
	}
	rl := ratelimit.NewRateLimiter(rateLimitInterval, options.RateLimitWait)
	chatOptions := routes.ChatCompletionOptions{
		RateLimiter:    rl,
		ManualApproval: options.Manual,
	}

	routes.ConfigureAdminStatus(chatOptions)

	// Set Gin mode
	if !options.Verbose {
		gin.SetMode(gin.ReleaseMode)
	}

	r := buildRouter(options.Port, options.APIKey, chatOptions)

	bindHost := options.Host
	if bindHost == "" {
		bindHost = "127.0.0.1"
	}
	bindAddr := fmt.Sprintf("%s:%d", bindHost, options.Port)

	// Claude Code mode output
	if options.ClaudeCode {
		serverURL := fmt.Sprintf("http://%s:%d", bindHost, options.Port)
		printClaudeCodeConfig(serverURL)
	}

	serverURL := fmt.Sprintf("http://%s:%d", bindHost, options.Port)
	adminURL := fmt.Sprintf("%s/admin", serverURL)

	log.Info().
		Str("url", serverURL).
		Str("admin", adminURL).
		Msg("OmniLLM server starting")

	log.Info().Str("api_key_path", filepath.Join(configDir, apiKeyFileName)).Msg("Inbound API authentication enabled")

	srv := &http.Server{
		Addr:    bindAddr,
		Handler: r.Handler(),
	}

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case err := <-serverErr:
		if err != nil {
			return err
		}
		return nil
	case sig := <-sigCh:
		log.Info().Str("signal", sig.String()).Msg("OmniLLM server shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}

	database.StopAsyncWorkers()
	if err := database.GetDatabase().Close(); err != nil {
		return fmt.Errorf("close database: %w", err)
	}
	return nil
}

func buildRouter(port int, apiKey string, chatOptions routes.ChatCompletionOptions) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	auth := newAuthConfig(apiKey)

	// Structured logging middleware with request ID
	r.Use(func(c *gin.Context) {
		requestID := generateRequestID()
		c.Set("request_id", requestID)
		c.Header("X-Request-Id", requestID)

		start := time.Now()
		c.Next()

		duration := time.Since(start)
		latencyMs := duration.Milliseconds()
		status := c.Writer.Status()

		log.Info().
			Str("request_id", requestID).
			Str("method", c.Request.Method).
			Str("path", c.Request.RequestURI).
			Int("status", status).
			Int64("latency_ms", latencyMs).
			Msg("HTTP")
	})

	// Configure CORS
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	corsConfig.AllowHeaders = []string{"Authorization", "Content-Type", "Accept", "Cache-Control"}
	corsConfig.AllowOriginFunc = isAllowedOrigin
	r.Use(cors.New(corsConfig))

	r.SetTrustedProxies([]string{"127.0.0.1", "::1", "localhost"})

	// EventSource middleware
	r.Use(func(c *gin.Context) {
		if c.GetHeader("Accept") == "text/event-stream" {
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-cache")
			c.Header("Connection", "keep-alive")
			if origin := c.GetHeader("Origin"); origin != "" && isAllowedOrigin(origin) {
				c.Header("Access-Control-Allow-Origin", origin)
			}
			c.Header("Access-Control-Allow-Headers", "Cache-Control, Authorization")
		}
		c.Next()
	})

	// Health check endpoints
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"message": "OmniLLM server is running",
			"version": routes.GetVersion(),
		})
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	r.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	// API routes
	api := r.Group("/", auth.middleware())
	routes.SetupChatCompletionRoutes(api, chatOptions)
	routes.SetupModelRoutes(api)
	routes.SetupEmbeddingRoutes(api)
	routes.SetupUsageRoutes(api)
	routes.SetupTokenRoutes(api)

	// v1 compatibility routes
	v1 := r.Group("/v1", auth.middleware())
	routes.SetupChatCompletionRoutes(v1, chatOptions)
	routes.SetupModelRoutes(v1)
	routes.SetupEmbeddingRoutes(v1)
	routes.SetupMessageRoutes(v1)
	routes.SetupResponseRoutes(v1)

	// Admin routes
	adminPublic := r.Group("/api/admin")
	adminPublic.GET("/info", routes.MakePublicInfoHandler(port))
	// OAuth callback and status must be public — Google's redirect carries no auth header,
	// and the status polling happens before the user has completed the flow.
	adminPublic.GET("/providers/antigravity/oauth-callback", routes.HandleAntigravityOAuthCallbackPublic)
	adminPublic.GET("/providers/antigravity/oauth-status", routes.HandleAntigravityOAuthStatusPublic)

	adminAPI := r.Group("/api/admin", auth.middleware())
	routes.SetupAdminRoutes(adminAPI, port)
	routes.SetupVirtualModelRoutes(adminAPI)

	// Admin static files redirect
	r.GET("/admin", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/admin/")
	})
	registerAdminUIRoutes(r, apiKey)

	return r
}

// generateRequestID creates a random request ID for correlation.
// hex.EncodeToString is ~5x faster than fmt.Sprintf("%x", b) for byte slices.
func generateRequestID() string {
	b := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		// Fallback to timestamp-based ID if RNG fails (should never happen on a properly functioning OS)
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func setupLogging(verbose bool) {
	var consoleWriter io.Writer = os.Stderr
	if verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	if verbose {
		consoleWriter = zerolog.ConsoleWriter{Out: os.Stderr}
	}

	log.Logger = log.Output(zerolog.MultiLevelWriter(
		consoleWriter,
		sseLogWriter{source: "backend"},
	))

	log.Info().Bool("verbose", verbose).Msg("Logging configured")
}

func registerDefaultProviders(reg *registry.ProviderRegistry, options StartOptions) error {
	// Skip if providers are already registered in memory (e.g., by tests)
	if len(reg.ListProviders()) > 0 {
		return nil
	}

	// Try to load saved provider instances from the database
	instanceStore := database.NewProviderInstanceStore()
	instances, err := instanceStore.GetAll()
	if err == nil && len(instances) > 0 {
		// Load providers from database
		for _, inst := range instances {
			var provider types.Provider
			switch inst.ProviderID {
			case "github-copilot":
				p := copilot.NewGitHubCopilotProvider(inst.InstanceID, inst.Name)
				if err := p.LoadFromDB(); err != nil {
					log.Warn().Err(err).Str("instance", inst.InstanceID).Msg("Failed to load provider token")
				}
				if options.GithubToken != "" {
					p.SetupAuth(&types.AuthOptions{GithubToken: options.GithubToken})
				}
				provider = p
			case "antigravity":
				p := antigravitypkg.NewProvider(inst.InstanceID, inst.Name)
				if err := p.LoadFromDB(); err != nil {
					log.Warn().Err(err).Str("instance", inst.InstanceID).Msg("Failed to load provider token")
				}
				provider = p
			case "alibaba":
				p := alibabapkg.NewProvider(inst.InstanceID, inst.Name)
				if err := p.LoadFromDB(); err != nil {
					log.Warn().Err(err).Str("instance", inst.InstanceID).Msg("Failed to load provider token")
				}
				provider = p
			case "alibaba-modelscope":
				p := modelscopepkg.NewProvider(inst.InstanceID, inst.Name)
				if err := p.LoadFromDB(); err != nil {
					log.Warn().Err(err).Str("instance", inst.InstanceID).Msg("Failed to load provider token")
				}
				provider = p
			case "azure-openai":
				p := azurepkg.NewProvider(inst.InstanceID, inst.Name)
				if err := p.LoadFromDB(); err != nil {
					log.Warn().Err(err).Str("instance", inst.InstanceID).Msg("Failed to load provider token")
				}
				provider = p
			case "google":
				p := googlepkg.NewProvider(inst.InstanceID, inst.Name)
				if err := p.LoadFromDB(); err != nil {
					log.Warn().Err(err).Str("instance", inst.InstanceID).Msg("Failed to load provider token")
				}
				provider = p
			case "openai-compatible":
				p := openaicompatprovider.NewProvider(inst.InstanceID, inst.Name)
				if err := p.LoadFromDB(); err != nil {
					log.Warn().Err(err).Str("instance", inst.InstanceID).Msg("Failed to load provider token")
				}
				provider = p
			case "codex":
				p := codexpkg.NewCodexProvider(inst.InstanceID)
				p.SetName(inst.Name)
				if err := p.LoadFromDB(); err != nil {
					log.Warn().Err(err).Str("instance", inst.InstanceID).Msg("Failed to load provider token")
				}
				provider = p
			case "kimi":
				p := kimipkg.NewProvider(inst.InstanceID, inst.Name)
				if err := p.LoadFromDB(); err != nil {
					log.Warn().Err(err).Str("instance", inst.InstanceID).Msg("Failed to load provider token")
				}
				provider = p
			default:
				log.Warn().Str("provider_id", inst.ProviderID).Str("instance", inst.InstanceID).Msg("Skipping unknown provider type during load")
				continue
			}

			if err := reg.Register(provider, false); err != nil {
				log.Warn().Err(err).Str("instance", inst.InstanceID).Msg("Failed to register provider")
				continue
			}
			if inst.Activated {
				reg.AddActive(inst.InstanceID)
			}
		}

		log.Info().Int("count", len(instances)).Msg("Loaded providers from database")
		return nil
	}

	// No saved providers - register default
	copilotProvider := copilot.NewGitHubCopilotProvider("github-copilot-1", "")

	// Try loading token from DB
	copilotProvider.LoadFromDB()

	// Override with CLI-provided token if given
	if options.GithubToken != "" {
		if err := copilotProvider.SetupAuth(&types.AuthOptions{GithubToken: options.GithubToken}); err != nil {
			log.Warn().Err(err).Msg("Failed to authenticate GitHub Copilot provider")
		}
	}

	if err := reg.Register(copilotProvider, false); err != nil {
		return fmt.Errorf("failed to register GitHub Copilot provider: %w", err)
	}

	// Always set copilot as active
	if _, err := reg.SetActive("github-copilot-1"); err != nil {
		log.Warn().Err(err).Msg("Failed to set GitHub Copilot as active provider")
	}

	log.Info().Msg("Default providers registered")
	return nil
}

func printClaudeCodeConfig(serverURL string) {
	fmt.Println("\n# Claude Code Configuration")
	fmt.Println("# Add these environment variables:")
	fmt.Printf("export OPENAI_API_KEY=dummy\n")
	fmt.Printf("export OPENAI_BASE_URL=%s/v1\n", serverURL)
	fmt.Printf("export ANTHROPIC_BASE_URL=%s/v1\n", serverURL)
	fmt.Println()
}
