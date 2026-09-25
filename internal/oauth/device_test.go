package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const deviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"

// deviceServer answers the device authorization request and then replies to
// each poll with the next response from polls (the last one repeats).
func deviceServer(t *testing.T, polls ...func() (int, map[string]any)) (*fakeServer, *atomic.Int32) {
	t.Helper()
	fs := newFakeServer(t)
	fs.setDevice(func(form url.Values) (int, map[string]any) {
		if form.Get("client_id") != "nimbu-cli" || form.Get("device_name") != "build-box" {
			t.Errorf("device authorization form = %v", form)
		}
		return http.StatusOK, map[string]any{
			"device_code":      "device-secret",
			"user_code":        "BCDF-GHJK",
			"verification_uri": "https://www.example.test/admin/oauth2/device",
			"expires_in":       600,
			"interval":         1,
		}
	})
	count := &atomic.Int32{}
	fs.setToken(func(form url.Values) (int, map[string]any) {
		if form.Get("grant_type") != deviceGrantType || form.Get("device_code") != "device-secret" {
			t.Errorf("device token form = %v", form)
		}
		n := int(count.Add(1)) - 1
		return polls[min(n, len(polls)-1)]()
	})
	return fs, count
}

func pollError(code string) func() (int, map[string]any) {
	return func() (int, map[string]any) {
		return http.StatusBadRequest, map[string]any{"error": code}
	}
}

func pollSuccess() (int, map[string]any) {
	return http.StatusOK, tokenResponse("access-1", "refresh-1")
}

func TestLoginWithDevicePollsUntilApproved(t *testing.T) {
	t.Parallel()
	fs, polls := deviceServer(t, pollError("authorization_pending"), pollSuccess)

	var shownURI, shownCode string
	tok, err := fs.client().LoginWithDevice(context.Background(), DeviceLogin{
		DeviceName: "build-box",
		Prompt: func(uri, code string, expiresAt time.Time) {
			shownURI, shownCode = uri, code
			if time.Until(expiresAt) < 9*time.Minute {
				t.Errorf("expiresAt = %v, want ~10 minutes out", expiresAt)
			}
		},
	})
	if err != nil {
		t.Fatalf("LoginWithDevice: %v", err)
	}
	if tok.AccessToken != "access-1" || polls.Load() != 2 {
		t.Fatalf("token = %+v after %d polls", tok, polls.Load())
	}
	if shownURI != "https://www.example.test/admin/oauth2/device" || shownCode != "BCDF-GHJK" {
		t.Fatalf("prompt = %q %q", shownURI, shownCode)
	}
}

func TestLoginWithDeviceBacksOffOnSlowDown(t *testing.T) {
	t.Parallel()
	fs, polls := deviceServer(t, pollError("slow_down"), pollSuccess)

	start := time.Now()
	if _, err := fs.client().LoginWithDevice(context.Background(), DeviceLogin{DeviceName: "build-box"}); err != nil {
		t.Fatalf("LoginWithDevice: %v", err)
	}
	// Interval 1s, then +5s after slow_down (RFC 8628 §3.5).
	if elapsed := time.Since(start); elapsed < 6*time.Second {
		t.Fatalf("second poll after %v, want the interval raised by 5s", elapsed)
	}
	if polls.Load() != 2 {
		t.Fatalf("polls = %d, want 2", polls.Load())
	}
}

func TestLoginWithDeviceTerminalErrors(t *testing.T) {
	t.Parallel()
	cases := map[string]error{
		"access_denied": ErrDeviceDenied,
		"expired_token": ErrDeviceExpired,
	}
	for code, want := range cases {
		t.Run(code, func(t *testing.T) {
			t.Parallel()
			fs, _ := deviceServer(t, pollError(code))
			_, err := fs.client().LoginWithDevice(context.Background(), DeviceLogin{DeviceName: "build-box"})
			if !errors.Is(err, want) {
				t.Fatalf("error = %v, want %v", err, want)
			}
		})
	}
}

// slowPoll answers after the client's per-request timeout has passed.
func slowPoll() (int, map[string]any) {
	time.Sleep(400 * time.Millisecond)
	return http.StatusBadRequest, map[string]any{"error": "authorization_pending"}
}

func TestLoginWithDeviceRetriesAPollThatTimedOut(t *testing.T) {
	t.Parallel()
	fs, polls := deviceServer(t, slowPoll, pollSuccess)
	client := fs.client()
	client.HTTPClient = &http.Client{Timeout: 100 * time.Millisecond}

	tok, err := client.LoginWithDevice(context.Background(), DeviceLogin{DeviceName: "build-box"})
	if err != nil {
		t.Fatalf("LoginWithDevice: %v", err)
	}
	if tok.AccessToken != "access-1" || polls.Load() != 2 {
		t.Fatalf("token = %+v after %d polls", tok, polls.Load())
	}
}

func TestLoginWithDevicePollTimeoutsAreNotReportedAsExpiredCode(t *testing.T) {
	t.Parallel()
	fs, polls := deviceServer(t, slowPoll)
	client := fs.client()
	client.HTTPClient = &http.Client{Timeout: 100 * time.Millisecond}

	_, err := client.LoginWithDevice(context.Background(), DeviceLogin{DeviceName: "build-box"})
	if err == nil || errors.Is(err, ErrDeviceExpired) {
		t.Fatalf("error = %v, want a transport error, not an expired code", err)
	}
	if got := polls.Load(); got != maxPollRetries+1 {
		t.Fatalf("polls = %d, want %d", got, maxPollRetries+1)
	}
}

// TestLoginWithDeviceKeepsSlowDownAcrossRetries: the server keeps its raised
// interval, so a retry after a transport error must not fall back to the
// original one (RFC 8628 §3.5).
func TestLoginWithDeviceKeepsSlowDownAcrossRetries(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var at []time.Time
	stamp := func(next func() (int, map[string]any)) func() (int, map[string]any) {
		return func() (int, map[string]any) {
			mu.Lock()
			at = append(at, time.Now())
			mu.Unlock()
			return next()
		}
	}
	fs, _ := deviceServer(t, stamp(pollError("slow_down")), stamp(slowPoll), stamp(pollSuccess))
	client := fs.client()
	client.HTTPClient = &http.Client{Timeout: 100 * time.Millisecond}

	if _, err := client.LoginWithDevice(context.Background(), DeviceLogin{DeviceName: "build-box"}); err != nil {
		t.Fatalf("LoginWithDevice: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(at) != 3 {
		t.Fatalf("polls = %d, want 3", len(at))
	}
	// Interval 1s + 5s after slow_down; the timed-out poll keeps it.
	if gap := at[2].Sub(at[1]); gap < 6*time.Second {
		t.Fatalf("retry after a timeout came %v later, want the raised 6s interval", gap)
	}
}

// TestLoginWithDeviceBoundsOnlyConsecutiveFailures: transport errors spread
// over the login, each followed by a server answer, do not end it.
func TestLoginWithDeviceBoundsOnlyConsecutiveFailures(t *testing.T) {
	t.Parallel()
	pending := pollError("authorization_pending")
	var polls []func() (int, map[string]any)
	for range maxPollRetries + 1 {
		polls = append(polls, slowPoll, pending)
	}
	fs, count := deviceServer(t, append(polls, pollSuccess)...)
	client := fs.client()
	client.HTTPClient = &http.Client{Timeout: 100 * time.Millisecond}

	tok, err := client.LoginWithDevice(context.Background(), DeviceLogin{DeviceName: "build-box"})
	if err != nil || tok.AccessToken != "access-1" {
		t.Fatalf("LoginWithDevice = %+v, %v after %d polls", tok, err, count.Load())
	}
}

// TestLoginWithDeviceExplainsALostApproval: when the poll that carried the
// approval is lost, the retry finds the code spent (invalid_grant).
func TestLoginWithDeviceExplainsALostApproval(t *testing.T) {
	t.Parallel()
	fs, _ := deviceServer(t, slowPoll, pollError("invalid_grant"))
	client := fs.client()
	client.HTTPClient = &http.Client{Timeout: 100 * time.Millisecond}

	_, err := client.LoginWithDevice(context.Background(), DeviceLogin{DeviceName: "build-box"})
	if !IsInvalidGrant(err) || !strings.Contains(err.Error(), "dropped response") {
		t.Fatalf("error = %v, want invalid_grant explaining the lost approval", err)
	}
}
