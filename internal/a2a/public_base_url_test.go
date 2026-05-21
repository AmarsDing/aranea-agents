package a2a

import "testing"

func TestResolvePublicBaseURL(t *testing.T) {
	t.Parallel()
	prefix := "/v1/a2a/public"

	t.Run("env wins", func(t *testing.T) {
		got := ResolvePublicBaseURL(PublicBaseURLInput{
			EnvOverride: "https://a2a.example.com/v1/a2a/public/",
			ConfigURL:   "http://ignored",
			HTTPAddr:    "0.0.0.0:8000",
			PathPrefix:  prefix,
		})
		if got.Source != PublicBaseSourceEnv || got.URL != "https://a2a.example.com/v1/a2a/public" {
			t.Fatalf("got %#v", got)
		}
	})

	t.Run("db second after env", func(t *testing.T) {
		got := ResolvePublicBaseURL(PublicBaseURLInput{
			DBURL:      "https://db.example/v1/a2a/public",
			ConfigURL:  "http://ignored",
			HTTPAddr:   ":9000",
			PathPrefix: prefix,
		})
		if got.Source != PublicBaseSourceDB || got.URL != "https://db.example/v1/a2a/public" {
			t.Fatalf("got %#v", got)
		}
	})

	t.Run("config third", func(t *testing.T) {
		got := ResolvePublicBaseURL(PublicBaseURLInput{
			ConfigURL:  "https://cfg.example/v1/a2a/public",
			HTTPAddr:   ":9000",
			PathPrefix: prefix,
		})
		if got.Source != PublicBaseSourceConfig {
			t.Fatalf("source=%q", got.Source)
		}
	})

	t.Run("derived colon port", func(t *testing.T) {
		got := ResolvePublicBaseURL(PublicBaseURLInput{HTTPAddr: ":8000", PathPrefix: prefix})
		if got.URL != "http://127.0.0.1:8000/v1/a2a/public" || got.Source != PublicBaseSourceDerived {
			t.Fatalf("got %#v", got)
		}
	})

	t.Run("derived zero bind", func(t *testing.T) {
		got := ResolvePublicBaseURL(PublicBaseURLInput{HTTPAddr: "0.0.0.0:8000", PathPrefix: prefix})
		if got.URL != "http://127.0.0.1:8000/v1/a2a/public" {
			t.Fatalf("got %#v", got)
		}
	})
}
