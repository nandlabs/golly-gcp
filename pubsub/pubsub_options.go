package pubsub

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"oss.nandlabs.io/golly/messaging"
)

// brokerOptions holds the Pub/Sub-relevant pieces parsed from the broker-targeted
// option keys introduced in golly v1.6.0 (DeliveryGuarantee, DeadLetter,
// MaxDeliveryAttempts, ConsumerMode, AckTimeout). It's the return value of
// applyBrokerOptions; wiring code inspects the flagged fields to decide what
// to do at each call site.
type brokerOptions struct {
	// maxExtension mirrors messaging.AckTimeoutOpt. Pub/Sub v2 exposes only a
	// receive-time knob for adjusting the ack-deadline lifecycle
	// (Subscription.AckDeadline is admin-plane, and this provider does not
	// own subscription lifecycle), so we translate AckTimeout into
	// Subscriber.ReceiveSettings.MaxExtension for the receive paths.
	maxExtension    time.Duration
	hasMaxExtension bool

	// exactlyOnce is set when DeliveryGuaranteeOpt == ExactlyOnce. Pub/Sub
	// only supports exactly-once when the subscription itself was created
	// with EnableExactlyOnceDelivery=true; we verify (not reconfigure) via
	// verifyExactlyOnce.
	exactlyOnce bool

	// verifyDeadLetter, deadLetterTopic, maxDeliveryAttempts mirror
	// DeadLetterOpt + MaxDeliveryAttemptsOpt. Pub/Sub only supports DLQ
	// configuration via the admin plane at subscription create/update time.
	// This provider verifies the existing subscription carries a matching
	// DeadLetterPolicy rather than silently reconfiguring the caller's sub.
	verifyDeadLetter    bool
	deadLetterTopic     string
	maxDeliveryAttempts int
}

// applyBrokerOptions parses the broker-targeted option keys defined in
// oss.nandlabs.io/golly/messaging (v1.6.0+). It performs syntactic and
// enum-level validation only — RPC-based verification (of DeadLetterPolicy or
// EnableExactlyOnceDelivery on the server-side subscription) is delegated to
// verifyDeadLetterPolicy / verifyExactlyOnce, which the receive paths invoke
// after fetching the subscription config.
//
// Returned errors indicate options that Pub/Sub cannot honor at all:
//   - DeliveryGuarantee=AtMostOnce (Pub/Sub is at-least-once minimum)
//   - ConsumerMode=Pull (this provider does not expose one-shot Pull; only
//     streaming-pull via Subscriber.Receive is wired)
//   - malformed values (wrong Go type, non-positive AckTimeout, empty
//     DeadLetter destination, non-positive MaxDeliveryAttempts, unknown enums)
//
// Callers on the publisher side may pass the parsed *brokerOptions through
// unchanged; the fields relevant to Send/SendBatch are limited to the
// enum-level rejections above.
func applyBrokerOptions(resolver *messaging.OptionsResolver) (*brokerOptions, error) {
	b := &brokerOptions{}

	if v, ok := resolver.Get(messaging.AckTimeoutOpt); ok {
		d, ok := v.(time.Duration)
		if !ok {
			return nil, fmt.Errorf("pubsub: %s must be time.Duration, got %T", messaging.AckTimeoutOpt, v)
		}
		if d <= 0 {
			return nil, fmt.Errorf("pubsub: %s must be > 0, got %s", messaging.AckTimeoutOpt, d)
		}
		b.maxExtension = d
		b.hasMaxExtension = true
	}

	if v, ok := resolver.Get(messaging.DeliveryGuaranteeOpt); ok {
		g, ok := v.(messaging.DeliveryGuarantee)
		if !ok {
			return nil, fmt.Errorf("pubsub: %s must be messaging.DeliveryGuarantee, got %T", messaging.DeliveryGuaranteeOpt, v)
		}
		switch g {
		case messaging.AtMostOnce:
			return nil, fmt.Errorf("pubsub: DeliveryGuarantee %q is not supported (Pub/Sub is at-least-once minimum)", g)
		case messaging.AtLeastOnce:
			// Default Pub/Sub semantic — no additional configuration required.
		case messaging.ExactlyOnce:
			b.exactlyOnce = true
		default:
			return nil, fmt.Errorf("pubsub: unknown DeliveryGuarantee %q", g)
		}
	}

	if v, ok := resolver.Get(messaging.ConsumerModeOpt); ok {
		m, ok := v.(messaging.ConsumerMode)
		if !ok {
			return nil, fmt.Errorf("pubsub: %s must be messaging.ConsumerMode, got %T", messaging.ConsumerModeOpt, v)
		}
		switch m {
		case messaging.ConsumerPush:
			// Pub/Sub Subscriber.Receive is streaming-pull that fans messages
			// out to the handler — effectively push-driven inside the client.
			// No extra config needed.
		case messaging.ConsumerPull:
			// The v2 Pub/Sub client does not expose a first-class one-shot
			// Pull API on Subscriber in this provider. Surface a clear error
			// rather than silently downgrading to streaming-pull.
			return nil, fmt.Errorf("pubsub: ConsumerMode %q is not supported by this provider (only streaming-pull via Subscriber.Receive is wired)", m)
		default:
			return nil, fmt.Errorf("pubsub: unknown ConsumerMode %q", m)
		}
	}

	if v, ok := resolver.Get(messaging.DeadLetterOpt); ok {
		dst, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("pubsub: %s must be a string (dead-letter topic name), got %T", messaging.DeadLetterOpt, v)
		}
		if dst == "" {
			return nil, fmt.Errorf("pubsub: %s must be a non-empty string", messaging.DeadLetterOpt)
		}
		b.deadLetterTopic = dst
		b.verifyDeadLetter = true
	}

	if v, ok := resolver.Get(messaging.MaxDeliveryAttemptsOpt); ok {
		n, ok := v.(int)
		if !ok {
			return nil, fmt.Errorf("pubsub: %s must be int, got %T", messaging.MaxDeliveryAttemptsOpt, v)
		}
		if n <= 0 {
			return nil, fmt.Errorf("pubsub: %s must be > 0, got %d", messaging.MaxDeliveryAttemptsOpt, n)
		}
		b.maxDeliveryAttempts = n
	}

	return b, nil
}

// verifyDeadLetterPolicy checks that the given subscription config already
// carries a DeadLetterPolicy matching the caller's DeadLetterOpt (and, when
// provided, MaxDeliveryAttemptsOpt). Pub/Sub only supports DLQ configuration
// via the admin plane at subscription create/update time. This provider does
// not own subscription lifecycle, so it verifies rather than reconfigures.
func verifyDeadLetterPolicy(sub *pubsubpb.Subscription, b *brokerOptions) error {
	if sub == nil {
		return fmt.Errorf("pubsub: subscription config unavailable; cannot verify DeadLetterPolicy")
	}
	dlp := sub.GetDeadLetterPolicy()
	if dlp == nil {
		return fmt.Errorf(
			"pubsub: subscription %q has no DeadLetterPolicy; create the subscription with a dead-letter topic (%q) before requesting the %s option",
			sub.GetName(), b.deadLetterTopic, messaging.DeadLetterOpt,
		)
	}
	if b.maxDeliveryAttempts > 0 && int(dlp.GetMaxDeliveryAttempts()) != b.maxDeliveryAttempts {
		return fmt.Errorf(
			"pubsub: subscription %q DeadLetterPolicy.MaxDeliveryAttempts=%d, requested %d; update the subscription's DeadLetterPolicy before requesting the %s option",
			sub.GetName(), dlp.GetMaxDeliveryAttempts(), b.maxDeliveryAttempts, messaging.MaxDeliveryAttemptsOpt,
		)
	}
	return nil
}

// verifyExactlyOnce checks that the given subscription config already has
// EnableExactlyOnceDelivery=true. Pub/Sub only allows toggling this via the
// admin plane at subscription create/update time.
func verifyExactlyOnce(sub *pubsubpb.Subscription) error {
	if sub == nil {
		return fmt.Errorf("pubsub: subscription config unavailable; cannot verify EnableExactlyOnceDelivery")
	}
	if !sub.GetEnableExactlyOnceDelivery() {
		return fmt.Errorf(
			"pubsub: subscription %q does not have EnableExactlyOnceDelivery set; create/update the subscription with exactly-once delivery before requesting DeliveryGuarantee=ExactlyOnce",
			sub.GetName(),
		)
	}
	return nil
}

// fetchSubscriptionConfig loads the admin-plane subscription config for
// server-side option verification (DeadLetterPolicy, EnableExactlyOnceDelivery).
// It's a thin wrapper around SubscriptionAdminClient.GetSubscription that uses
// the same context as the calling receive path.
func fetchSubscriptionConfig(ctx context.Context, client *pubsub.Client, sub *pubsub.Subscriber) (*pubsubpb.Subscription, error) {
	if client == nil || sub == nil {
		return nil, fmt.Errorf("pubsub: nil client or subscriber")
	}
	got, err := client.SubscriptionAdminClient.GetSubscription(ctx, &pubsubpb.GetSubscriptionRequest{
		Subscription: sub.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("pubsub: fetch subscription config: %w", err)
	}
	return got, nil
}

// enforceServerSideBrokerOptions applies the receive-time-relevant broker
// options to a *pubsub.Subscriber and runs the admin-plane verifications
// (DLQ + exactly-once) when the corresponding options were requested. It is
// called from AddListener and ReceiveBatch immediately after they resolve
// their subscriber.
func enforceServerSideBrokerOptions(ctx context.Context, client *pubsub.Client, sub *pubsub.Subscriber, b *brokerOptions) error {
	if b == nil {
		return nil
	}
	if b.hasMaxExtension {
		sub.ReceiveSettings.MaxExtension = b.maxExtension
	}
	if !b.exactlyOnce && !b.verifyDeadLetter {
		return nil
	}
	cfg, err := fetchSubscriptionConfig(ctx, client, sub)
	if err != nil {
		return err
	}
	if b.exactlyOnce {
		if err := verifyExactlyOnce(cfg); err != nil {
			return err
		}
	}
	if b.verifyDeadLetter {
		if err := verifyDeadLetterPolicy(cfg, b); err != nil {
			return err
		}
	}
	return nil
}
