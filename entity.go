package discoverkit

import (
	"context"

	"github.com/dogmatiq/configkit"
)

// application is an implementation of config.Application.
type application struct {
	IdentityValue     configkit.Identity
	TypeNameValue     string
	MessageNamesValue configkit.EntityMessageNames
	HandlersValue     configkit.HandlerSet
}

// Identity returns the identity of the entity.
func (a *application) Identity() configkit.Identity {
	return a.IdentityValue
}

// TypeName returns the fully-qualified type name of the entity.
func (a *application) TypeName() string {
	return a.TypeNameValue
}

// MessageNames returns information about the messages used by the entity.
func (a *application) MessageNames() configkit.EntityMessageNames {
	return a.MessageNamesValue
}

// Handlers returns the handlers within this application.
func (a *application) Handlers() configkit.HandlerSet {
	return a.HandlersValue
}

// AcceptVisitor calls the appropriate method on v for this entity type.
func (a *application) AcceptVisitor(ctx context.Context, v configkit.Visitor) error {
	return v.VisitApplication(ctx, a)
}

// handler is an implementation of config.Handler.
type handler struct {
	IdentityValue     configkit.Identity
	TypeNameValue     string
	MessageNamesValue configkit.EntityMessageNames
	HandlerTypeValue  configkit.HandlerType
}

// Identity returns the identity of the entity.
func (h *handler) Identity() configkit.Identity {
	return h.IdentityValue
}

// TypeName returns the fully-qualified type name of the entity.
func (h *handler) TypeName() string {
	return h.TypeNameValue
}

// MessageNames returns information about the messages used by the entity.
func (h *handler) MessageNames() configkit.EntityMessageNames {
	return h.MessageNamesValue
}

// HandlerType returns the type of handler.
func (h *handler) HandlerType() configkit.HandlerType {
	return h.HandlerTypeValue
}

// AcceptVisitor calls the appropriate method on v for this entity type.
func (h *handler) AcceptVisitor(ctx context.Context, v configkit.Visitor) error {
	h.HandlerTypeValue.MustValidate()

	switch h.HandlerTypeValue {
	case configkit.AggregateHandlerType:
		return v.VisitAggregate(ctx, h)
	case configkit.ProcessHandlerType:
		return v.VisitProcess(ctx, h)
	case configkit.IntegrationHandlerType:
		return v.VisitIntegration(ctx, h)
	default: // configkit.ProjectionHandlerType
		return v.VisitProjection(ctx, h)
	}
}
