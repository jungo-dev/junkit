package i18n_test

import (
	"testing"

	"github.com/jungo-dev/junkit/i18n"
)

func TestNewTranslator_SeedsBuiltInCatalog(t *testing.T) {
	tr := i18n.NewTranslator()

	if got := tr.GetDefaultLang(); got != i18n.LangEN {
		t.Fatalf("GetDefaultLang() = %q, want %q", got, i18n.LangEN)
	}
	if got := tr.GetMessage("required", i18n.LangEN, "email"); got != "email is required" {
		t.Fatalf(`GetMessage("required", "en", "email") = %q, want %q`, got, "email is required")
	}
}

func TestTranslator_SetDefaultLang(t *testing.T) {
	tr := i18n.NewTranslator()

	tr.SetDefaultLang(i18n.LangVI)
	if got := tr.GetDefaultLang(); got != i18n.LangVI {
		t.Fatalf("GetDefaultLang() = %q, want %q", got, i18n.LangVI)
	}

	tr.SetDefaultLang("")
	if got := tr.GetDefaultLang(); got != i18n.LangEN {
		t.Fatalf(`SetDefaultLang("") should reset to %q, got %q`, i18n.LangEN, got)
	}
}

func TestTranslator_AddTranslations(t *testing.T) {
	tr := i18n.NewTranslator()

	tr.AddTranslations(map[string]map[string]string{
		i18n.LangEN: {"user_not_found": "User not found"},
		i18n.LangVI: {"user_not_found": "Không tìm thấy người dùng"},
	})

	if got := tr.GetMessage("user_not_found", i18n.LangEN); got != "User not found" {
		t.Fatalf("GetMessage(en) = %q, want %q", got, "User not found")
	}
	if got := tr.GetMessage("user_not_found", i18n.LangVI); got != "Không tìm thấy người dùng" {
		t.Fatalf("GetMessage(vi) = %q, want %q", got, "Không tìm thấy người dùng")
	}

	t.Run("existing keys are overwritten, not merged away", func(t *testing.T) {
		tr.AddTranslations(map[string]map[string]string{
			i18n.LangEN: {"user_not_found": "No such user"},
		})
		if got := tr.GetMessage("user_not_found", i18n.LangEN); got != "No such user" {
			t.Fatalf("GetMessage(en) after overwrite = %q, want %q", got, "No such user")
		}
		// The vi entry, untouched by this call, must survive.
		if got := tr.GetMessage("user_not_found", i18n.LangVI); got != "Không tìm thấy người dùng" {
			t.Fatalf("GetMessage(vi) = %q, want it unaffected by an en-only update", got)
		}
	})
}

func TestTranslator_GetMessage(t *testing.T) {
	tr := i18n.NewTranslator()
	tr.AddTranslations(map[string]map[string]string{
		i18n.LangEN: {"greeting": "Hello, %s!"},
		i18n.LangVI: {"greeting": "Xin chào, %s!"},
	})

	t.Run("resolves in the requested language", func(t *testing.T) {
		if got := tr.GetMessage("greeting", i18n.LangVI, "Lan"); got != "Xin chào, Lan!" {
			t.Fatalf("GetMessage(vi) = %q, want %q", got, "Xin chào, Lan!")
		}
	})

	t.Run("empty lang uses the default language", func(t *testing.T) {
		if got := tr.GetMessage("greeting", "", "Jane"); got != "Hello, Jane!" {
			t.Fatalf("GetMessage(\"\") = %q, want the default-language message", got)
		}
	})

	t.Run("falls back to the default language when missing from the requested one", func(t *testing.T) {
		if got := tr.GetMessage("greeting", "fr", "Marie"); got != "Hello, Marie!" {
			t.Fatalf("GetMessage(fr) = %q, want it to fall back to en", got)
		}
	})

	t.Run("an entirely unknown key returns the key itself", func(t *testing.T) {
		if got := tr.GetMessage("no_such_key", i18n.LangEN); got != "no_such_key" {
			t.Fatalf("GetMessage(unknown key) = %q, want the key echoed back", got)
		}
	})

	t.Run("a message with no verbs is returned as-is even with args", func(t *testing.T) {
		tr.AddTranslations(map[string]map[string]string{i18n.LangEN: {"static": "a plain message"}})
		if got := tr.GetMessage("static", i18n.LangEN, "ignored"); got != "a plain message" {
			t.Fatalf("GetMessage(static) = %q, want the message unformatted", got)
		}
	})
}

func TestTranslator_ConcurrentAccess(t *testing.T) {
	tr := i18n.NewTranslator()
	done := make(chan struct{})

	go func() {
		for i := 0; i < 100; i++ {
			tr.AddTranslations(map[string]map[string]string{i18n.LangEN: {"k": "v"}})
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		tr.GetMessage("k", i18n.LangEN)
	}
	<-done
}
