// Copyright Istio Authors
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

package manifestcustomization

import "strings"

// ParsePath parses a field path string into path components, respecting selector boundaries.
// This is needed because util.PathFromString() naively splits on '.' even inside selectors,
// which breaks selectors that contain dots in their values.
func ParsePath(fieldPath string) []string {
	var parts []string
	var current strings.Builder
	inSelector := false

	for _, ch := range fieldPath {
		switch ch {
		case '[':
			// Start of selector
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
			inSelector = true
			current.WriteRune(ch)
		case ']':
			// End of selector
			current.WriteRune(ch)
			parts = append(parts, current.String())
			current.Reset()
			inSelector = false
		case '.':
			if inSelector {
				// Inside selector, keep the dot
				current.WriteRune(ch)
			} else if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}

	// Add remaining part
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}
