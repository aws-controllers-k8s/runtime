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

package errors

import (
	"fmt"
)

var (
	// FieldExportPathDoesNotExist indicates the path specified in the field
	// export spec does not exist on the object
	FieldExportPathDoesNotExist = fmt.Errorf("path does not exist in this object")
	// FieldExportResourceNotSynced indicates the source resource does not have
	// the ResourceSynced condition set to True
	FieldExportResourceNotSynced = fmt.Errorf("the source resource is not synced yet")
	// FieldExportInvalidPath indicates there was an error parsing the path into
	// a JQ query
	FieldExportInvalidPath = TerminalError{err: fmt.Errorf("unable to parse path")}
	// FieldExportInvalidPath indicates there was an error when executing the
	// JQ query
	FieldExportQueryFailed = TerminalError{err: fmt.Errorf("unable to execute query")}
	// FieldExportMissingConfigMap indicates there was an error when trying
	// to get the target configmap
	FieldExportMissingConfigMap = fmt.Errorf("unable to get existing configmap")
	// FieldExportMissingSecret indicates there was an error when trying
	// to get the target secret
	FieldExportMissingSecret = fmt.Errorf("unable to get existing secret")
)
