package validate

import (
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en"
)

type Validator struct {
	validate *validator.Validate
	trans    ut.Translator
}

func New() (*Validator, error) {
	validate := validator.New(validator.WithRequiredStructEnabled())

	enLocale := en.New()
	uni := ut.New(enLocale, enLocale)
	trans, ok := uni.GetTranslator("en")
	if !ok {
		return nil, fmt.Errorf("failed to get English translator")
	}

	if err := en_translations.RegisterDefaultTranslations(validate, trans); err != nil {
		return nil, fmt.Errorf("failed to register translations: %w", err)
	}

	return &Validator{
		validate: validate,
		trans:    trans,
	}, nil
}

// Validate validates a struct and returns a Connect error if validation fails.
func (v *Validator) Validate(s any) error {
	err := v.validate.Struct(s)
	if err == nil {
		return nil
	}

	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return connect.NewError(connect.CodeInternal, err)
	}

	messages := make([]string, 0, len(validationErrors))
	for _, e := range validationErrors {
		messages = append(messages, e.Translate(v.trans))
	}

	return connect.NewError(connect.CodeInvalidArgument, errors.New(strings.Join(messages, "; ")))
}
