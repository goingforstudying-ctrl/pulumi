// Copyright 2026, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package httpstate

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi/pkg/v3/engine"
	pkgresource "github.com/pulumi/pulumi/pkg/v3/resource"
	"github.com/pulumi/pulumi/pkg/v3/resource/stack"
	"github.com/pulumi/pulumi/pkg/v3/secrets"
	"github.com/pulumi/pulumi/pkg/v3/secrets/b64"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
)

func outputsEvent(urn resource.URN, typ tokens.Type, outputs resource.PropertyMap) engine.Event {
	return engine.NewEvent(engine.ResourceOutputsEventPayload{
		Metadata: engine.StepEventMetadata{
			URN:  urn,
			Type: typ,
			New: &engine.StepEventStateMetadata{
				URN:     urn,
				Type:    typ,
				State:   &pkgresource.State{URN: urn, Type: typ, Outputs: outputs},
				Outputs: outputs,
			},
		},
	})
}

func TestStackOutputsRecorder(t *testing.T) {
	t.Parallel()

	stackURN := resource.NewURN("stack", "project", "", resource.RootStackType, "project-stack")
	otherURN := resource.NewURN("stack", "project", "", "pkg:index:Thing", "thing")

	t.Run("records the last stack outputs event", func(t *testing.T) {
		t.Parallel()
		r := &stackOutputsRecorder{}
		r.record(outputsEvent(otherURN, "pkg:index:Thing", resource.PropertyMap{
			"ignored": resource.NewProperty("me"),
		}))
		r.record(outputsEvent(stackURN, resource.RootStackType, resource.PropertyMap{
			"partial": resource.NewProperty("value"),
		}))
		r.record(outputsEvent(stackURN, resource.RootStackType, resource.PropertyMap{
			"plain":  resource.NewProperty("value"),
			"secret": resource.MakeSecret(resource.NewProperty("hunter2")),
		}))

		sm := b64.NewBase64SecretsManager()
		resp, err := r.response(t.Context(), sm)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, b64.Type, resp.SecretsProviders.Type)

		secretsProvider := (&secrets.MockProvider{}).Add(b64.Type, func(json.RawMessage) (secrets.Manager, error) {
			return b64.NewBase64SecretsManager(), nil
		})
		roundTripped, err := stack.DecryptStackOutputs(
			t.Context(), resp.Outputs, resp.SecretsProviders, secretsProvider)
		require.NoError(t, err)
		assert.Equal(t, resource.PropertyMap{
			"plain":  resource.NewProperty("value"),
			"secret": resource.MakeSecret(resource.NewProperty("hunter2")),
		}, roundTripped)
	})

	t.Run("reports nothing when the stack never registered outputs", func(t *testing.T) {
		t.Parallel()
		r := &stackOutputsRecorder{}
		r.record(outputsEvent(otherURN, "pkg:index:Thing", resource.PropertyMap{}))

		resp, err := r.response(t.Context(), b64.NewBase64SecretsManager())
		require.NoError(t, err)
		assert.Nil(t, resp)
	})

	t.Run("a nil recorder is inert", func(t *testing.T) {
		t.Parallel()
		var r *stackOutputsRecorder
		r.record(outputsEvent(stackURN, resource.RootStackType, resource.PropertyMap{}))

		resp, err := r.response(t.Context(), b64.NewBase64SecretsManager())
		require.NoError(t, err)
		assert.Nil(t, resp)
	})
}
