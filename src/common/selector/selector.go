/*
 * Tencent is pleased to support the open source community by making 蓝鲸 available.,
 * Copyright (C) 2017,-2018 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the ",License",); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an ",AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */
package selector

import (
	"errors"
	"fmt"

	"configcenter/src/common"
	"configcenter/src/common/util"
)

type LabelAddOption struct {
	InstanceIDs []int64 `json:"instance_ids"`
	Labels      Labels  `json:"labels"`
}

type LabelAddRequest struct {
	Option    LabelAddOption `json:"option"`
	TableName string         `json:"table_name"`
}

type LabelRemoveOption struct {
	InstanceIDs []int64  `json:"instance_ids"`
	Keys        []string `json:"keys"`
}

type LabelRemoveRequest struct {
	Option    LabelRemoveOption `json:"option"`
	TableName string            `json:"table_name"`
}

type Operator string

const (
	DoesNotExist Operator = "!"
	Equals       Operator = "="
	In           Operator = "in"
	NotEquals    Operator = "!="
	NotIn        Operator = "notin"
	Exists       Operator = "exists"
)

var AvailableOperators = []Operator{
	DoesNotExist,
	Equals,
	In,
	NotEquals,
	NotIn,
	Exists,
}

type Selector struct {
	key      string
	operator Operator
	values   []string
}

func (s *Selector) Validate() (string, error) {
	if util.InArray(s.operator, AvailableOperators) == false {
		return "operator", fmt.Errorf("operator %s not available, available operators: %+v", s.operator, AvailableOperators)
	}

	if (s.operator == In || s.operator == NotIn) && len(s.values) == 0 {
		return "values", errors.New("values shouldn't be empty")
	}

	if (s.operator == Exists || s.operator == DoesNotExist) && len(s.values) > 0 {
		return "values", errors.New("values shouldn be empty")
	}

	if (s.operator == Equals || s.operator == NotEquals) && len(s.values) != 1 {
		return "values", errors.New("values field length for equal operation should exactly one")
	}

	if LabelNGKeyRule.MatchString(s.key) == false {
		return "key", fmt.Errorf("key %s invalid", s.key)
	}
	return "", nil
}

func (s *Selector) ToMgoFilter() (map[string]interface{}, error) {
	filter := make(map[string]interface{})
	field := "labels." + s.key
	switch s.operator {
	case In:
		filter = map[string]interface{}{
			field: map[string]interface{}{
				common.BKDBIN: s.values,
			},
		}
	case NotIn:
		filter = map[string]interface{}{
			field: map[string]interface{}{
				common.BKDBNIN: s.values,
			},
		}
	case DoesNotExist, Exists:
		filter = map[string]interface{}{
			field: map[string]interface{}{
				common.BKDBExists: s.operator == Exists,
			},
		}
	case Equals:
		if len(s.values) == 0 {
			return nil, errors.New("values empty")
		}
		firstValue := s.values[0]
		filter = map[string]interface{}{
			field: firstValue,
		}
	case NotEquals:
		if len(s.values) == 0 {
			return nil, errors.New("values empty")
		}
		firstValue := s.values[0]
		filter = map[string]interface{}{
			field: map[string]interface{}{
				common.BKDBNot: firstValue,
			},
		}
	}
	return filter, nil
}

type Selectors []Selector

func (ss Selectors) Validate() (string, error) {
	for _, selector := range ss {
		if key, err := selector.Validate(); err != nil {
			return key, err
		}
	}
	return "", nil
}

func (ss Selectors) ToMgoFilter() (map[string]interface{}, error) {
	filters := make([]map[string]interface{}, 0)
	for _, selector := range ss {
		filter, err := selector.ToMgoFilter()
		if err != nil {
			return nil, err
		}
		filters = append(filters, filter)
	}
	return util.MergeMaps(filters...), nil
}
