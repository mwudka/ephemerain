package main

import (
	"context"
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
)

func TestDelete(t *testing.T) {
	ctx := context.Background()
	err := withRedisTestServer(ctx, func(port int) {
		registrar := NewRedisRegistrar("localhost:" + strconv.Itoa(port))

		value := "baz"
		fqdn := Domain("foo.bar.")
		err := registrar.SetRecord(ctx, fqdn, RecordTypeTXT, value)
		assert.NoError(t, err)

		record, err := registrar.GetRecord(ctx, fqdn, RecordTypeTXT)
		assert.NoError(t, err)
		assert.Equal(t, value, record)

		// Deleting with the wrong current value should fail
		err = registrar.DeleteRecord(ctx, fqdn, RecordTypeTXT, "wrongvalue")
		assert.Error(t, err)

		// The deletion should fail, so the record should still be present
		record, err = registrar.GetRecord(ctx, fqdn, RecordTypeTXT)
		assert.NoError(t, err)
		assert.Equal(t, value, record)

		// Deleting with the correct current value should succeed
		err = registrar.DeleteRecord(ctx, fqdn, RecordTypeTXT, value)
		assert.NoError(t, err)

		// And since the deletion should now succeed, the record should be gone
		_, err = registrar.GetRecord(ctx, fqdn, RecordTypeTXT)
		assert.Error(t, err)
	})

	assert.NoError(t, err)
}
