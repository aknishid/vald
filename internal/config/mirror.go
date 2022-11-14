//
// Copyright (C) 2019-2022 vdaas.org vald team <vald@vdaas.org>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

// Package config providers configuration type and load configuration logic
package config

// Mirror represents the Mirror Gateway configuration.
type Mirror struct {
	// Client represent the gRPC client configuration for connecting the other Mirror Gateway.
	Client *GRPCClient `json:"client" yaml:"client"`
}

// Bind binds the actual data from the Mirror receiver fields.
func (m *Mirror) Bind() *Mirror {
	if m.Client != nil {
		m.Client = m.Client.Bind()
	} else {
		m.Client = new(GRPCClient).Bind()
	}
	return m
}
