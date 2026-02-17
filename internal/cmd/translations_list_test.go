package cmd

import (
	"reflect"
	"testing"

	"github.com/nimbu/cli/internal/api"
)

func TestExpandTranslationsListRowsExpandsValues(t *testing.T) {
	translations := []api.Translation{
		{
			Key: "account.addresses.delete",
			Values: map[string]string{
				"nl": "Adres verwijderen",
				"en": "Delete address",
			},
		},
	}

	got := expandTranslationsListRows(translations, "")
	want := []api.Translation{
		{Key: "account.addresses.delete", Locale: "en", Value: "Delete address"},
		{Key: "account.addresses.delete", Locale: "nl", Value: "Adres verwijderen"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rows: %#v", got)
	}
}

func TestExpandTranslationsListRowsRespectsLocaleFilter(t *testing.T) {
	translations := []api.Translation{
		{
			Key: "account.addresses.delete",
			Values: map[string]string{
				"en": "Delete address",
				"fr": "Supprimer l'adresse",
			},
		},
	}

	got := expandTranslationsListRows(translations, "fr")
	want := []api.Translation{{Key: "account.addresses.delete", Locale: "fr", Value: "Supprimer l'adresse"}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rows: %#v", got)
	}
}

func TestExpandTranslationsListRowsRespectsLocaleFilterBaseLanguage(t *testing.T) {
	translations := []api.Translation{
		{
			Key: "account.addresses.delete",
			Values: map[string]string{
				"nl-BE": "Adres verwijderen",
			},
		},
	}

	got := expandTranslationsListRows(translations, "nl")
	want := []api.Translation{{Key: "account.addresses.delete", Locale: "nl", Value: "Adres verwijderen"}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected rows: %#v", got)
	}
}

func TestTranslationValueForLocaleNormalizesUnderscore(t *testing.T) {
	value, ok := translationValueForLocale(map[string]string{"nl-BE": "Adres verwijderen"}, "nl_BE")
	if !ok {
		t.Fatal("expected locale match")
	}
	if value != "Adres verwijderen" {
		t.Fatalf("unexpected value: %q", value)
	}
}

func TestExpandTranslationsListRowsKeepsExistingLocaleValue(t *testing.T) {
	translations := []api.Translation{{
		Key:    "account.addresses.delete",
		Locale: "en",
		Value:  "Delete address",
		Values: map[string]string{"fr": "Supprimer l'adresse"},
	}}

	got := expandTranslationsListRows(translations, "")

	if !reflect.DeepEqual(got, translations) {
		t.Fatalf("unexpected rows: %#v", got)
	}
}
