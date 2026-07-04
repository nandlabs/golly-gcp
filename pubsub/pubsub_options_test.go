package pubsub

import (
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"oss.nandlabs.io/golly/messaging"
)

// TestPubSub_AckTimeout_ConfiguresMaxExtension verifies that
// messaging.AckTimeoutOpt (v1.6.0 broker option) is parsed into the
// brokerOptions.maxExtension knob that receive paths apply to
// Subscriber.ReceiveSettings.MaxExtension.
func TestPubSub_AckTimeout_ConfiguresMaxExtension(t *testing.T) {
	want := 45 * time.Second
	resolver := messaging.NewOptionsResolver(messaging.Option{
		Key:   messaging.AckTimeoutOpt,
		Value: want,
	})

	got, err := applyBrokerOptions(resolver)
	if err != nil {
		t.Fatalf("applyBrokerOptions returned unexpected error: %v", err)
	}
	if !got.hasMaxExtension {
		t.Fatal("expected hasMaxExtension=true, got false")
	}
	if got.maxExtension != want {
		t.Errorf("expected maxExtension=%s, got %s", want, got.maxExtension)
	}
}

// TestPubSub_AckTimeout_RejectsNonPositive guards against callers passing a
// zero/negative duration, which would collapse ack-extension in the Pub/Sub
// subscriber and cause silent redeliveries.
func TestPubSub_AckTimeout_RejectsNonPositive(t *testing.T) {
	resolver := messaging.NewOptionsResolver(messaging.Option{
		Key:   messaging.AckTimeoutOpt,
		Value: time.Duration(0),
	})
	if _, err := applyBrokerOptions(resolver); err == nil {
		t.Fatal("expected error for AckTimeout=0")
	}
}

// TestPubSub_AckTimeout_RejectsWrongType guards against option-type drift.
func TestPubSub_AckTimeout_RejectsWrongType(t *testing.T) {
	resolver := messaging.NewOptionsResolver(messaging.Option{
		Key:   messaging.AckTimeoutOpt,
		Value: 30, // int, not time.Duration
	})
	if _, err := applyBrokerOptions(resolver); err == nil {
		t.Fatal("expected error for AckTimeout with wrong type")
	}
}

// TestPubSub_ExactlyOnce_EnablesFlag verifies that DeliveryGuarantee=ExactlyOnce
// is recorded on the returned brokerOptions so the receive path can verify the
// subscription's server-side EnableExactlyOnceDelivery flag.
func TestPubSub_ExactlyOnce_EnablesFlag(t *testing.T) {
	resolver := messaging.NewOptionsResolver(messaging.Option{
		Key:   messaging.DeliveryGuaranteeOpt,
		Value: messaging.ExactlyOnce,
	})

	got, err := applyBrokerOptions(resolver)
	if err != nil {
		t.Fatalf("applyBrokerOptions returned unexpected error: %v", err)
	}
	if !got.exactlyOnce {
		t.Fatal("expected exactlyOnce=true, got false")
	}
}

// TestPubSub_ExactlyOnce_VerifiesSubscription checks that verifyExactlyOnce
// errors when the server-side subscription lacks EnableExactlyOnceDelivery and
// succeeds when it's set.
func TestPubSub_ExactlyOnce_VerifiesSubscription(t *testing.T) {
	// Server-side flag off — should error.
	subOff := &pubsubpb.Subscription{
		Name:                      "projects/p/subscriptions/s",
		EnableExactlyOnceDelivery: false,
	}
	if err := verifyExactlyOnce(subOff); err == nil {
		t.Fatal("expected error when EnableExactlyOnceDelivery=false")
	} else if !strings.Contains(err.Error(), "EnableExactlyOnceDelivery") {
		t.Errorf("error should reference EnableExactlyOnceDelivery, got: %v", err)
	}

	// Server-side flag on — should succeed.
	subOn := &pubsubpb.Subscription{
		Name:                      "projects/p/subscriptions/s",
		EnableExactlyOnceDelivery: true,
	}
	if err := verifyExactlyOnce(subOn); err != nil {
		t.Errorf("expected nil error when EnableExactlyOnceDelivery=true, got %v", err)
	}
}

// TestPubSub_DeliveryGuarantee_RejectsAtMostOnce verifies Pub/Sub rejects
// AtMostOnce (which it cannot honor — Pub/Sub is at-least-once minimum).
func TestPubSub_DeliveryGuarantee_RejectsAtMostOnce(t *testing.T) {
	resolver := messaging.NewOptionsResolver(messaging.Option{
		Key:   messaging.DeliveryGuaranteeOpt,
		Value: messaging.AtMostOnce,
	})

	_, err := applyBrokerOptions(resolver)
	if err == nil {
		t.Fatal("expected error for DeliveryGuarantee=AtMostOnce, got nil")
	}
	if !strings.Contains(err.Error(), "AtMostOnce") && !strings.Contains(err.Error(), "at-most-once") {
		t.Errorf("error should mention AtMostOnce, got: %v", err)
	}
}

// TestPubSub_DeliveryGuarantee_AcceptsAtLeastOnce verifies the default Pub/Sub
// semantic is honored as a no-op (no exactly-once flag).
func TestPubSub_DeliveryGuarantee_AcceptsAtLeastOnce(t *testing.T) {
	resolver := messaging.NewOptionsResolver(messaging.Option{
		Key:   messaging.DeliveryGuaranteeOpt,
		Value: messaging.AtLeastOnce,
	})

	got, err := applyBrokerOptions(resolver)
	if err != nil {
		t.Fatalf("expected AtLeastOnce accepted, got error: %v", err)
	}
	if got.exactlyOnce {
		t.Error("expected exactlyOnce=false for AtLeastOnce")
	}
}

// TestPubSub_ConsumerMode_RejectsPullIfUnsupported verifies the provider
// surfaces a clear error for ConsumerPull rather than silently downgrading
// (this provider only exposes the streaming-pull path via Subscriber.Receive).
func TestPubSub_ConsumerMode_RejectsPullIfUnsupported(t *testing.T) {
	resolver := messaging.NewOptionsResolver(messaging.Option{
		Key:   messaging.ConsumerModeOpt,
		Value: messaging.ConsumerPull,
	})

	_, err := applyBrokerOptions(resolver)
	if err == nil {
		t.Fatal("expected error for ConsumerMode=Pull, got nil")
	}
	if !strings.Contains(err.Error(), "pull") && !strings.Contains(err.Error(), "Pull") {
		t.Errorf("error should reference pull mode, got: %v", err)
	}
}

// TestPubSub_ConsumerMode_AcceptsPush verifies ConsumerPush is a no-op
// (streaming-pull inside Subscriber.Receive is effectively push-driven).
func TestPubSub_ConsumerMode_AcceptsPush(t *testing.T) {
	resolver := messaging.NewOptionsResolver(messaging.Option{
		Key:   messaging.ConsumerModeOpt,
		Value: messaging.ConsumerPush,
	})
	if _, err := applyBrokerOptions(resolver); err != nil {
		t.Errorf("expected ConsumerPush accepted, got error: %v", err)
	}
}

// TestPubSub_DeadLetter_RequiresPreconfiguredPolicy verifies the provider
// refuses to run when DeadLetter is requested but the server-side subscription
// has no DeadLetterPolicy — the operator must create the subscription with the
// DLQ set, this provider does not reconfigure it.
func TestPubSub_DeadLetter_RequiresPreconfiguredPolicy(t *testing.T) {
	// Parse succeeds (syntactic check only).
	resolver := messaging.NewOptionsResolver(messaging.Option{
		Key:   messaging.DeadLetterOpt,
		Value: "projects/p/topics/my-dlq",
	})
	b, err := applyBrokerOptions(resolver)
	if err != nil {
		t.Fatalf("applyBrokerOptions returned unexpected error: %v", err)
	}
	if !b.verifyDeadLetter {
		t.Fatal("expected verifyDeadLetter=true")
	}
	if b.deadLetterTopic != "projects/p/topics/my-dlq" {
		t.Errorf("expected deadLetterTopic captured, got %q", b.deadLetterTopic)
	}

	// Verification against a subscription config with no DeadLetterPolicy
	// must fail with a helpful message.
	subNoDLQ := &pubsubpb.Subscription{
		Name: "projects/p/subscriptions/s",
	}
	err = verifyDeadLetterPolicy(subNoDLQ, b)
	if err == nil {
		t.Fatal("expected error when subscription has no DeadLetterPolicy")
	}
	if !strings.Contains(err.Error(), "DeadLetterPolicy") {
		t.Errorf("error should reference DeadLetterPolicy, got: %v", err)
	}
	if !strings.Contains(err.Error(), "create the subscription") {
		t.Errorf("error should tell the caller to create the subscription with DLQ, got: %v", err)
	}

	// Verification against a subscription that already has a matching DLQ
	// policy must succeed.
	subWithDLQ := &pubsubpb.Subscription{
		Name: "projects/p/subscriptions/s",
		DeadLetterPolicy: &pubsubpb.DeadLetterPolicy{
			DeadLetterTopic:     "projects/p/topics/my-dlq",
			MaxDeliveryAttempts: 5,
		},
	}
	if err := verifyDeadLetterPolicy(subWithDLQ, b); err != nil {
		t.Errorf("expected nil error for preconfigured DLQ, got %v", err)
	}
}

// TestPubSub_DeadLetter_RejectsEmptyDestination guards against operators
// passing an empty string as the DLQ destination.
func TestPubSub_DeadLetter_RejectsEmptyDestination(t *testing.T) {
	resolver := messaging.NewOptionsResolver(messaging.Option{
		Key:   messaging.DeadLetterOpt,
		Value: "",
	})
	if _, err := applyBrokerOptions(resolver); err == nil {
		t.Fatal("expected error for empty DeadLetter destination")
	}
}

// TestPubSub_MaxDeliveryAttempts_Parses verifies MaxDeliveryAttemptsOpt is
// captured on the returned brokerOptions and, together with DeadLetter,
// participates in the DLQ verification.
func TestPubSub_MaxDeliveryAttempts_Parses(t *testing.T) {
	resolver := messaging.NewOptionsResolver(
		messaging.Option{Key: messaging.DeadLetterOpt, Value: "projects/p/topics/dlq"},
		messaging.Option{Key: messaging.MaxDeliveryAttemptsOpt, Value: 7},
	)
	b, err := applyBrokerOptions(resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.maxDeliveryAttempts != 7 {
		t.Errorf("expected maxDeliveryAttempts=7, got %d", b.maxDeliveryAttempts)
	}

	// Mismatched server-side MaxDeliveryAttempts must be surfaced as an error.
	subMismatch := &pubsubpb.Subscription{
		Name: "projects/p/subscriptions/s",
		DeadLetterPolicy: &pubsubpb.DeadLetterPolicy{
			DeadLetterTopic:     "projects/p/topics/dlq",
			MaxDeliveryAttempts: 3,
		},
	}
	if err := verifyDeadLetterPolicy(subMismatch, b); err == nil {
		t.Fatal("expected error for MaxDeliveryAttempts mismatch")
	}
}

// TestPubSub_MaxDeliveryAttempts_RejectsNonPositive guards against callers
// passing zero/negative attempts.
func TestPubSub_MaxDeliveryAttempts_RejectsNonPositive(t *testing.T) {
	resolver := messaging.NewOptionsResolver(messaging.Option{
		Key:   messaging.MaxDeliveryAttemptsOpt,
		Value: 0,
	})
	if _, err := applyBrokerOptions(resolver); err == nil {
		t.Fatal("expected error for MaxDeliveryAttempts=0")
	}
}

// TestPubSub_NoBrokerOptions_ReturnsEmptyStruct verifies that a resolver with
// no broker-targeted options returns a zero-valued *brokerOptions and nil
// error — matching the "advisory hint" contract from the golly messaging pkg.
func TestPubSub_NoBrokerOptions_ReturnsEmptyStruct(t *testing.T) {
	resolver := messaging.NewOptionsResolver()
	b, err := applyBrokerOptions(resolver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil brokerOptions")
	}
	if b.hasMaxExtension || b.exactlyOnce || b.verifyDeadLetter || b.maxDeliveryAttempts != 0 {
		t.Errorf("expected zero-valued brokerOptions, got %+v", b)
	}
}
