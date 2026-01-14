package validate

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"connectrpc.com/connect"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en"
)

// dns1123LabelRegex matches valid Kubernetes namespace/DNS-1123 label names.
// Must start with lowercase letter, end with alphanumeric, contain only lowercase alphanumerics and hyphens.
var dns1123LabelRegex = regexp.MustCompile(`^[a-z]([-a-z0-9]*[a-z0-9])?$`)

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

	// Register custom dns1123label validator for Kubernetes resource names
	if err := validate.RegisterValidation("dns1123label", validateDNS1123Label); err != nil {
		return nil, fmt.Errorf("failed to register dns1123label validator: %w", err)
	}

	// Register translation for dns1123label
	if err := validate.RegisterTranslation("dns1123label", trans,
		func(ut ut.Translator) error {
			return ut.Add("dns1123label", "{0} must start with a lowercase letter, end with a letter or number, and contain only lowercase letters, numbers, and hyphens", true)
		},
		func(ut ut.Translator, fe validator.FieldError) string {
			t, _ := ut.T("dns1123label", fe.Field())
			return t
		},
	); err != nil {
		return nil, fmt.Errorf("failed to register dns1123label translation: %w", err)
	}

	return &Validator{
		validate: validate,
		trans:    trans,
	}, nil
}

// validateDNS1123Label validates that a string is a valid DNS-1123 label (Kubernetes namespace name).
func validateDNS1123Label(fl validator.FieldLevel) bool {
	return dns1123LabelRegex.MatchString(fl.Field().String())
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
