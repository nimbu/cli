package migrate

import (
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestSharedNonDefaultLocalesExcludesBothSiteDefaults(t *testing.T) {
	got := sharedNonDefaultLocales(
		[]string{"nl", "en", "fr", "de"},
		[]string{"en", "fr", "de"},
		"nl",
		"en",
	)
	want := []string{"de", "fr"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestChannelsHaveLocalizedFields(t *testing.T) {
	channelMap := map[string]api.ChannelDetail{
		"plain": {
			Slug:           "plain",
			Customizations: []api.CustomField{{Name: "title", Type: "text"}},
		},
		"localized": {
			Slug:           "localized",
			Customizations: []api.CustomField{{Name: "title", Type: "text", Localized: true}},
		},
	}
	if channelsHaveLocalizedFields(channelMap, []string{"plain"}) {
		t.Fatal("did not expect localized fields for plain channel")
	}
	if !channelsHaveLocalizedFields(channelMap, []string{"plain", "localized"}) {
		t.Fatal("expected localized fields for selected channel set")
	}
}
