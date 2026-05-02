package routes

import (
	"net/http"
	"omnillm/internal/database"
	"omnillm/internal/lib/modelrouting"
	"omnillm/internal/lib/virtualmodelrouting"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func SetupVirtualModelRoutes(router *gin.RouterGroup) {
	router.GET("/virtualmodels", handleListVirtualModels)
	router.POST("/virtualmodels", handleCreateVirtualModel)
	router.GET("/virtualmodels/:id", handleGetVirtualModel)
	router.PUT("/virtualmodels/:id", handleUpdateVirtualModel)
	router.POST("/virtualmodels/:id/rename", handleRenameVirtualModel)
	router.DELETE("/virtualmodels/:id", handleDeleteVirtualModel)
}

// ─── request / response types ─────────────────────────────────────────────────

type upstreamInput struct {
	ProviderID string `json:"provider_id"`
	ModelID    string `json:"model_id"  binding:"required"`
	Weight     int    `json:"weight"`
	Priority   int    `json:"priority"`
}

type virtualModelPayload struct {
	VirtualModelID string          `json:"virtual_model_id" binding:"required"`
	Name           string          `json:"name"             binding:"required"`
	Description    string          `json:"description"`
	APIShape       string          `json:"api_shape"`
	LbStrategy     string          `json:"lb_strategy"      binding:"required"`
	Enabled        *bool           `json:"enabled"`
	Upstreams      []upstreamInput `json:"upstreams"`
}

type virtualModelResponse struct {
	database.VirtualModelRecord
	Upstreams []database.VirtualModelUpstreamRecord `json:"upstreams"`
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func vmWithUpstreams(vm *database.VirtualModelRecord) (*virtualModelResponse, error) {
	us := database.NewVirtualModelUpstreamStore()
	upstreams, err := us.GetForVModel(vm.VirtualModelID)
	if err != nil {
		return nil, err
	}
	if upstreams == nil {
		upstreams = []database.VirtualModelUpstreamRecord{}
	}
	return &virtualModelResponse{VirtualModelRecord: *vm, Upstreams: upstreams}, nil
}

func saveUpstreams(virtualModelID string, inputs []upstreamInput) error {
	us := database.NewVirtualModelUpstreamStore()
	records := make([]database.VirtualModelUpstreamRecord, 0, len(inputs))
	for _, u := range inputs {
		w := u.Weight
		if w < 1 {
			w = 1
		}

		// When a provider_id is present, store the model_id with the provider
		// prefix so the upstream record is self-describing and collisions between
		// providers that expose the same model name are avoided.
		// e.g.  provider_id="alibaba-2", model_id="deepseek-v4-flash"
		//   →   stored model_id="alipay01/deepseek-v4-flash" (or instanceID/...)
		modelID := u.ModelID
		if u.ProviderID != "" {
			// Use the subtitle if one is set, otherwise fall back to the instance ID.
			prefix := providerDisplayPrefix(u.ProviderID)
			existingPrefix, _ := modelrouting.ParseProviderPrefix(modelID)
			if existingPrefix == "" {
				modelID = prefix + "/" + modelID
			}
		}

		records = append(records, database.VirtualModelUpstreamRecord{
			VirtualModelID: virtualModelID,
			ProviderID:     u.ProviderID,
			ModelID:        modelID,
			Weight:         w,
			Priority:       u.Priority,
		})
	}
	return us.SetForVModel(virtualModelID, records)
}

// providerDisplayPrefix returns the subtitle for the given instance ID if one
// is set, otherwise the instance ID itself. This is used as the prefix stored
// alongside the model ID in virtual-model upstream records.
func providerDisplayPrefix(instanceID string) string {
	instanceStore := database.NewProviderInstanceStore()
	record, err := instanceStore.Get(instanceID)
	if err != nil || record == nil {
		return instanceID
	}
	if record.Subtitle != "" {
		return record.Subtitle
	}
	return instanceID
}

// ─── handlers ─────────────────────────────────────────────────────────────────

func handleListVirtualModels(c *gin.Context) {
	store := database.NewVirtualModelStore()
	virtualmodels, err := store.GetAll()
	if err != nil {
		log.Error().Err(err).Msg("Failed to list virtual models")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list virtual models"})
		return
	}

	result := make([]virtualModelResponse, 0, len(virtualmodels))
	for i := range virtualmodels {
		resp, err := vmWithUpstreams(&virtualmodels[i])
		if err != nil {
			log.Warn().Err(err).Str("id", virtualmodels[i].VirtualModelID).Msg("Failed to load upstreams")
			continue
		}
		result = append(result, *resp)
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func handleGetVirtualModel(c *gin.Context) {
	id := c.Param("id")
	store := database.NewVirtualModelStore()
	vm, err := store.Get(id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to get virtual model")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get virtual model"})
		return
	}
	if vm == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Virtual model not found"})
		return
	}
	resp, err := vmWithUpstreams(vm)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load upstreams"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func handleCreateVirtualModel(c *gin.Context) {
	var payload virtualModelPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	apiShape := payload.APIShape
	if apiShape == "" {
		apiShape = "openai"
	}
	enabled := true
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}

	store := database.NewVirtualModelStore()
	// Check for duplicate
	existing, _ := store.Get(payload.VirtualModelID)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Virtual model ID already exists"})
		return
	}

	record := &database.VirtualModelRecord{
		VirtualModelID: payload.VirtualModelID,
		Name:           payload.Name,
		Description:    payload.Description,
		APIShape:       apiShape,
		LbStrategy:     database.LbStrategy(payload.LbStrategy),
		Enabled:        enabled,
	}
	if err := store.Create(record); err != nil {
		log.Error().Err(err).Msg("Failed to create virtual model")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create virtual model"})
		return
	}

	if err := saveUpstreams(payload.VirtualModelID, payload.Upstreams); err != nil {
		log.Error().Err(err).Msg("Failed to save upstreams")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save upstreams"})
		return
	}

	vm, _ := store.Get(payload.VirtualModelID)
	resp, _ := vmWithUpstreams(vm)
	c.JSON(http.StatusCreated, resp)
}

func handleUpdateVirtualModel(c *gin.Context) {
	id := c.Param("id")
	var payload virtualModelPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	store := database.NewVirtualModelStore()
	vm, err := store.Get(id)
	if err != nil || vm == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Virtual model not found"})
		return
	}

	vm.Name = payload.Name
	vm.Description = payload.Description
	if payload.APIShape != "" {
		vm.APIShape = payload.APIShape
	}
	vm.LbStrategy = database.LbStrategy(payload.LbStrategy)
	if payload.Enabled != nil {
		vm.Enabled = *payload.Enabled
	}

	if err := store.Update(vm); err != nil {
		log.Error().Err(err).Msg("Failed to update virtual model")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update virtual model"})
		return
	}

	if err := saveUpstreams(id, payload.Upstreams); err != nil {
		log.Error().Err(err).Msg("Failed to save upstreams")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save upstreams"})
		return
	}

	updated, _ := store.Get(id)
	resp, _ := vmWithUpstreams(updated)
	c.JSON(http.StatusOK, resp)
}

type renamePayload struct {
	NewID string `json:"new_id" binding:"required"`
}

func handleRenameVirtualModel(c *gin.Context) {
	oldID := c.Param("id")

	var payload renamePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new_id is required"})
		return
	}

	newID := strings.TrimSpace(payload.NewID)
	if newID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new_id cannot be empty"})
		return
	}

	store := database.NewVirtualModelStore()
	if err := store.Rename(oldID, newID); err != nil {
		msg := err.Error()
		switch {
		case msg == "virtual model not found":
			c.JSON(http.StatusNotFound, gin.H{"error": "Virtual model not found"})
		case msg == "virtual model ID already exists":
			c.JSON(http.StatusConflict, gin.H{"error": "Virtual model ID already exists"})
		default:
			log.Error().Err(err).Str("old_id", oldID).Str("new_id", newID).Msg("Failed to rename virtual model")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to rename virtual model"})
		}
		return
	}

	virtualmodelrouting.MigrateRoundRobinCursor(oldID, newID)

	vm, _ := store.Get(newID)
	resp, _ := vmWithUpstreams(vm)
	c.JSON(http.StatusOK, resp)
}

func handleDeleteVirtualModel(c *gin.Context) {
	id := c.Param("id")
	store := database.NewVirtualModelStore()
	vm, err := store.Get(id)
	if err != nil || vm == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Virtual model not found"})
		return
	}
	if err := store.Delete(id); err != nil {
		log.Error().Err(err).Msg("Failed to delete virtual model")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete virtual model"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}
