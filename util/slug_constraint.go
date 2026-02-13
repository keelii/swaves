package util

import "github.com/gofiber/fiber/v2"

type CustomConstraint interface {
	// Name returns the name of the constraint.
	// This name is used in the constraint matching.
	Name() string

	// Execute executes the constraint.
	// It returns true if the constraint is matched and right.
	// param is the parameter value to check.
	// args are the constraint arguments.
	Execute(param string, args ...string) bool
}

type SlugConstraint struct {
	fiber.Constraint
}

func (*SlugConstraint) Name() string {
	return "ulid"
}

func (*SlugConstraint) Execute(param string, args ...string) bool {
	_, err := ulid.Parse(param)
	return err == nil
}
