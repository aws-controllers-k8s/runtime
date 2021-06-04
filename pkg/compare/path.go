// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package compare

import (
	"encoding/json"
	"strings"
)

// Path provides a JSONPath-like struct and field-member "route" to a
// particular field within a compared struct. Path implements json.Marshaler
// interface.
type Path struct {
	parts []string
}

// MarshalJSON returns the JSON encoding of a Path object.
func (p Path) MarshalJSON() ([]byte, error) {
	// Since json.Marshall doesn't encode unexported struct fields we have to
	// copy the Path instance into a new struct object with exported fields.
	// See https://github.com/aws-controllers-k8s/community/issues/772
	return json.Marshal(
		struct {
			Parts []string
		}{
			p.parts,
		},
	)
}

// Push adds a new part to the Path.
func (p Path) Push(part string) {
	p.parts = append(p.parts, part)
}

// Pop removes the last part from the Path
func (p Path) Pop() {
	if len(p.parts) > 0 {
		p.parts = p.parts[:len(p.parts)-1]
	}
}

//	Contains returns true if the supplied string, delimited on ".", matches
//	p.parts up to the length of the supplied string.
//		e.g. if the Path p represents "A.B":
//			subject "A" -> true
//			subject "A.B" -> true
//			subject "A.B.C" -> false
//			subject "B" -> false
//			subject "A.C" -> false
func (p Path) Contains(subject string) bool {
	subjectSplit := strings.Split(subject, ".")

	if len(subjectSplit) > len(p.parts) {
		return false
	}

	for i, s := range subjectSplit {
		if p.parts[i] != s {
			return false
		}
	}

	return true
}

// NewPath returns a new Path struct pointer from a dotted-notation string,
// e.g. "Author.Name"
func NewPath(dotted string) Path {
	return Path{strings.Split(dotted, ".")}
}
