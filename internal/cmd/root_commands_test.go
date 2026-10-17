package cmd

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestCLIHasNewTopLevelCommands(t *testing.T) {
	rt := reflect.TypeOf(CLI{})
	required := []string{
		"Collections",
		"Coupons",
		"Domains",
		"Init",
		"Mails",
		"Notifications",
		"Roles",
		"Redirects",
		"Senders",
		"Functions",
		"Jobs",
		"Apps",
		"Server",
	}

	for _, field := range required {
		if _, ok := rt.FieldByName(field); !ok {
			t.Fatalf("CLI missing %s command", field)
		}
	}
}

func TestWebhooksCmdOnlyExposesSupportedSubcommands(t *testing.T) {
	rt := reflect.TypeOf(WebhooksCmd{})
	required := []string{"List", "Get", "Delete"}
	for _, field := range required {
		if _, ok := rt.FieldByName(field); !ok {
			t.Fatalf("WebhooksCmd missing %s command", field)
		}
	}

	for _, field := range []string{"Create", "Update", "Count"} {
		if _, ok := rt.FieldByName(field); ok {
			t.Fatalf("WebhooksCmd unexpectedly exposes %s", field)
		}
	}
}

func TestReadmeMentionsNewTopLevelCommands(t *testing.T) {
	data, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}

	readme := string(data)
	required := []string{
		"nimbu collections",
		"nimbu coupons",
		"nimbu domains",
		"nimbu init",
		"nimbu mails",
		"nimbu notifications",
		"nimbu roles",
		"nimbu redirects",
		"nimbu senders",
		"nimbu functions",
		"nimbu jobs",
		"nimbu apps",
		"nimbu apps push",
		"nimbu apps logs",
		"nimbu apps code pull",
		"nimbu server",
		"nimbu themes pull",
		"nimbu themes diff",
		"nimbu themes cdn-root",
		"nimbu themes copy",
		"nimbu themes push",
		"nimbu themes sync",
	}

	for _, needle := range required {
		if !strings.Contains(readme, needle) {
			t.Fatalf("README missing command entry: %s", needle)
		}
	}
}

func TestReadmeDocumentsInlinePayloadSyntax(t *testing.T) {
	data, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}

	readme := string(data)
	required := []string{
		"## Inline Payload Syntax",
		"key=value",
		"key:=json",
		"key=@file.txt",
		"key:=@file.json",
		"translations update --key activate.label.lastname nl=Achternaam",
	}

	for _, needle := range required {
		if !strings.Contains(readme, needle) {
			t.Fatalf("README missing inline payload docs: %s", needle)
		}
	}
}

func TestParserBuildsWithoutDuplicateFlags(t *testing.T) {
	if _, _, err := newParser(); err != nil {
		t.Fatalf("newParser() error = %v", err)
	}
}

func TestOrdersCmdExposesAdminActionSubcommands(t *testing.T) {
	rt := reflect.TypeOf(OrdersCmd{})
	required := []string{"Pay", "Finish", "Cancel", "Reopen", "Archive"}
	for _, field := range required {
		if _, ok := rt.FieldByName(field); !ok {
			t.Fatalf("OrdersCmd missing %s command", field)
		}
	}
}

func TestCustomersCmdExposesAdminActionSubcommands(t *testing.T) {
	rt := reflect.TypeOf(CustomersCmd{})
	required := []string{"ResetPassword", "ResendConfirmation"}
	for _, field := range required {
		if _, ok := rt.FieldByName(field); !ok {
			t.Fatalf("CustomersCmd missing %s command", field)
		}
	}
}

func TestChannelsCmdExposesEmptySubcommand(t *testing.T) {
	rt := reflect.TypeOf(ChannelsCmd{})
	if _, ok := rt.FieldByName("Empty"); !ok {
		t.Fatal("ChannelsCmd missing Empty command")
	}
}
