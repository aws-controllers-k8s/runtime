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

// Difference contains the difference in values for a specified field path into
// two compared resources.
type Difference struct {
	// Path is the field path to the detected difference between resources
	// under comparison
	Path Path
	// A is the value of the first resource under comparison at the Path
	A interface{}
	// B is the value of the first resource under comparison at the Path
	B interface{}
}
