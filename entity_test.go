package discoverkit

import (
	"context"
	"errors"

	"github.com/dogmatiq/configkit"
	. "github.com/dogmatiq/configkit/fixtures"
	"github.com/dogmatiq/configkit/message"
	"github.com/dogmatiq/dogma"
	. "github.com/dogmatiq/dogma/fixtures"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("type application", func() {
	var app *application

	BeforeEach(func() {
		app = &application{}
	})

	Describe("func Identity()", func() {
		It("returns the value of .IdentityValue field", func() {
			i := configkit.MustNewIdentity("<name>", "<key>")
			app.IdentityValue = i
			Expect(app.Identity()).To(Equal(i))
		})
	})

	Describe("func TypeName()", func() {
		It("returns the value of .TypeNameValue field", func() {
			app.TypeNameValue = "<type>"
			Expect(app.TypeName()).To(Equal("<type>"))
		})
	})

	Describe("func MessageNames()", func() {
		It("returns the value of .MessageNamesValue field", func() {
			m := configkit.EntityMessageNames{
				Produced: message.NameRoles{
					MessageCTypeName: message.CommandRole,
				},
				Consumed: message.NameRoles{
					MessageETypeName: message.EventRole,
				},
			}

			app.MessageNamesValue = m
			Expect(app.MessageNames()).To(Equal(m))
		})
	})

	Describe("func AcceptVisitor()", func() {
		It("calls the correct visitor method", func() {
			v := &Visitor{
				VisitApplicationFunc: func(_ context.Context, a configkit.Application) error {
					Expect(a).To(Equal(app))
					return errors.New("<error>")
				},
			}

			err := app.AcceptVisitor(context.Background(), v)
			Expect(err).To(Equal(err))
		})
	})

	Describe("func Handlers()", func() {
		It("returns the value of .HandlersValue field", func() {
			aggregate := configkit.FromAggregate(&AggregateMessageHandler{
				ConfigureFunc: func(c dogma.AggregateConfigurer) {
					c.Identity("<agg-name>", "<agg-key>")
					c.ConsumesCommandType(MessageC{})
					c.ProducesEventType(MessageE{})
				},
			})

			projection := configkit.FromProjection(&ProjectionMessageHandler{
				ConfigureFunc: func(c dogma.ProjectionConfigurer) {
					c.Identity("<proj-name>", "<proj-key>")
					c.ConsumesEventType(MessageE{})
				},
			})

			s := configkit.NewHandlerSet(aggregate, projection)

			app.HandlersValue = s
			Expect(app.Handlers()).To(Equal(s))

		})
	})
})

var _ = Describe("type handler", func() {
	var hnd *handler

	BeforeEach(func() {
		hnd = &handler{}
	})

	Describe("func Identity()", func() {
		It("returns the value of .IdentityValue field", func() {
			i := configkit.MustNewIdentity("<name>", "<key>")
			hnd.IdentityValue = i
			Expect(hnd.Identity()).To(Equal(i))
		})
	})

	Describe("func TypeName()", func() {
		It("returns the value of .TypeNameValue field", func() {
			hnd.TypeNameValue = "<type>"
			Expect(hnd.TypeName()).To(Equal("<type>"))
		})
	})

	Describe("func MessageNames()", func() {
		It("returns the value of .MessageNamesValue field", func() {
			m := configkit.EntityMessageNames{
				Produced: message.NameRoles{
					MessageCTypeName: message.CommandRole,
				},
				Consumed: message.NameRoles{
					MessageETypeName: message.EventRole,
				},
			}

			hnd.MessageNamesValue = m
			Expect(hnd.MessageNames()).To(Equal(m))
		})
	})

	Describe("func HandlerType()", func() {
		It("returns the value of .HandlerTypeValue field", func() {
			hnd.HandlerTypeValue = configkit.AggregateHandlerType
			Expect(hnd.HandlerType()).To(Equal(configkit.AggregateHandlerType))
		})
	})

	Describe("func AcceptVisitor()", func() {
		var visitor *Visitor

		BeforeEach(func() {
			visitor = &Visitor{}
		})

		When("the handler is an aggregate", func() {
			BeforeEach(func() {
				hnd.HandlerTypeValue = configkit.AggregateHandlerType
			})

			It("calls the correct visitor method", func() {
				visitor.VisitAggregateFunc = func(_ context.Context, h configkit.Aggregate) error {
					Expect(h).To(Equal(hnd))
					return errors.New("<error>")
				}

				err := hnd.AcceptVisitor(context.Background(), visitor)
				Expect(err).To(Equal(err))
			})
		})

		When("the handler is a process", func() {
			BeforeEach(func() {
				hnd.HandlerTypeValue = configkit.ProcessHandlerType
			})

			It("calls the correct visitor method", func() {
				visitor.VisitProcessFunc = func(_ context.Context, h configkit.Process) error {
					Expect(h).To(Equal(hnd))
					return errors.New("<error>")
				}

				err := hnd.AcceptVisitor(context.Background(), visitor)
				Expect(err).To(Equal(err))
			})
		})

		When("the handler is an integration", func() {
			BeforeEach(func() {
				hnd.HandlerTypeValue = configkit.IntegrationHandlerType
			})

			It("calls the correct visitor method", func() {
				visitor.VisitIntegrationFunc = func(_ context.Context, h configkit.Integration) error {
					Expect(h).To(Equal(hnd))
					return errors.New("<error>")
				}

				err := hnd.AcceptVisitor(context.Background(), visitor)
				Expect(err).To(Equal(err))
			})
		})

		When("the handler is a projection", func() {
			BeforeEach(func() {
				hnd.HandlerTypeValue = configkit.ProjectionHandlerType
			})

			It("calls the correct visitor method", func() {
				visitor.VisitProjectionFunc = func(_ context.Context, h configkit.Projection) error {
					Expect(h).To(Equal(hnd))
					return errors.New("<error>")
				}

				err := hnd.AcceptVisitor(context.Background(), visitor)
				Expect(err).To(Equal(err))
			})
		})
	})
})
