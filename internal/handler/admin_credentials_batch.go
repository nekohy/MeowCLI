package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/nekohy/MeowCLI/utils"

	"github.com/gin-gonic/gin"
)

type credentialStatusUpdateFunc func(context.Context, string, string, string) error
type credentialDeleteFunc func(context.Context, string) error

func (a *AdminHandler) batchUpdateCredentialStatus(c *gin.Context, handler utils.HandlerType, update credentialStatusUpdateFunc) {
	var req batchUpdateStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	updated := make([]string, 0, len(req.IDs))
	errs := make([]batchError, 0)
	for _, id := range req.IDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if err := update(ctx, id, req.Status, ""); err != nil {
			errs = append(errs, batchError{
				Input: id,
				Error: storeErrorMessage(err, "credential not found", ""),
			})
			continue
		}
		updated = append(updated, id)
	}

	a.refreshCredentials(ctx, handler, updated)
	if req.Status == "enabled" {
		a.syncCredentialQuotas(ctx, handler, updated)
	}
	c.JSON(http.StatusOK, gin.H{
		"updated": updated,
		"errors":  errs,
	})
}

func (a *AdminHandler) batchDeleteCredentials(c *gin.Context, handler utils.HandlerType, deleteCredential credentialDeleteFunc) {
	var req batchDeleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	deleted := make([]string, 0, len(req.IDs))
	errs := make([]batchError, 0)
	for _, id := range req.IDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if err := deleteCredential(ctx, id); err != nil {
			errs = append(errs, batchError{
				Input: id,
				Error: storeErrorMessage(err, "credential not found", ""),
			})
			continue
		}
		deleted = append(deleted, id)
	}

	a.refreshCredentials(ctx, handler, deleted)
	c.JSON(http.StatusOK, gin.H{
		"deleted": deleted,
		"errors":  errs,
	})
}
