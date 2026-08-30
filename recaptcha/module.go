package recaptcha

import (
	"net/http"

	"go.uber.org/fx"
)

// Module provides an Fx provider for reCAPTCHA Verifier.
//
// Usage:
//
//	fx.New(
//	    fx.Supply(recaptcha.Options{SecretKey: cfg.RecaptchaSecret, Mock: cfg.Environment != "production"}),
//	    httpclient.Module,
//	    recaptcha.Module,
//	)
var Module = fx.Module("recaptcha",
	fx.Provide(newVerifier),
)

// newVerifier constructs a Verifier (Client or MockVerifier) based on opts.Mock.
func newVerifier(opts Options, httpClient *http.Client) Verifier {
	if opts.Mock {
		return NewMockVerifier()
	}
	return NewClient(opts, httpClient)
}
