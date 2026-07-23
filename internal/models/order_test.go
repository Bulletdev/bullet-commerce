package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrderStatus_ValidTransitions(t *testing.T) {
	cases := []struct {
		from OrderStatus
		to   OrderStatus
	}{
		{StatusPending, StatusProcessing},
		{StatusPending, StatusCancelled},
		{StatusProcessing, StatusShipped},
		{StatusProcessing, StatusCancelled},
		{StatusShipped, StatusDelivered},
	}

	for _, tc := range cases {
		assert.True(t, tc.from.CanTransitionTo(tc.to),
			"%s → %s should be allowed", tc.from, tc.to)
	}
}

func TestOrderStatus_InvalidTransitions(t *testing.T) {
	cases := []struct {
		from OrderStatus
		to   OrderStatus
	}{
		{StatusPending, StatusShipped},
		{StatusPending, StatusDelivered},
		{StatusProcessing, StatusPending},
		{StatusShipped, StatusPending},
		{StatusShipped, StatusProcessing},
		{StatusShipped, StatusCancelled},
		{StatusDelivered, StatusCancelled},
		{StatusDelivered, StatusProcessing},
		{StatusCancelled, StatusPending},
		{StatusCancelled, StatusProcessing},
	}

	for _, tc := range cases {
		assert.False(t, tc.from.CanTransitionTo(tc.to),
			"%s → %s should be forbidden", tc.from, tc.to)
	}
}

func TestOrderStatus_UnknownStatus(t *testing.T) {
	unknown := OrderStatus("unknown")
	assert.False(t, unknown.CanTransitionTo(StatusProcessing))
}
