/*
 * Tencent is pleased to support the open source community by making 蓝鲸 available.
 * Copyright (C) 2017-2018 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package logics

import (
	"context"
	"fmt"
	"net/http"

	"configcenter/src/common"
	"configcenter/src/common/blog"
	"configcenter/src/common/mapstr"
	meta "configcenter/src/common/metadata"
	"configcenter/src/common/util"
)

// AddDevice create new net device
func (lgc *Logics) AddDevice(header http.Header, deviceInfo meta.NetcollectDevice) (meta.AddDeviceResult, error) {
	deviceID, err := lgc.addDevice(header, deviceInfo, util.GetOwnerID(header))
	if nil != err {
		return meta.AddDeviceResult{DeviceID: INVALIDID}, err
	}

	return meta.AddDeviceResult{DeviceID: deviceID}, nil
}

func (lgc *Logics) UpdateDevice(pheader http.Header, netDeviceID uint64, deviceInfo meta.NetcollectDevice) error {
	defErr := lgc.Engine.CCErr.CreateDefaultCCErrorIf(util.GetLanguage(pheader))

	// check device id if exists or not
	if _, _, err := lgc.checkNetDeviceExist(pheader, netDeviceID, ""); nil != err {
		switch err.Error() {
		case defErr.Error(common.CCErrCollectNetDeviceGetFail).Error():
			blog.Errorf("[NetDevice] update net device, net device does not exist, deviceID: [%d]", netDeviceID)
		default:
			blog.Errorf("[NetDevice] update net device fail, error: %v, deviceID: [%d]", err, netDeviceID)
		}
		return err
	}

	// check if device name is empty
	if "" == deviceInfo.DeviceName {
		blog.Errorf("[NetDevice] update net device fail, device name is empty string")
		return defErr.Errorf(common.CCErrCommParamsNeedSet, common.BKDeviceNameField)
	}
	// check device name has been occupied or not
	deviceID, _, err := lgc.checkNetDeviceExist(pheader, 0, deviceInfo.DeviceName)
	if nil != err && err.Error() != defErr.Error(common.CCErrCollectNetDeviceGetFail).Error() {
		blog.Errorf("[NetDevice] update net device fail, error: %v, deviceID: [%d]", err, netDeviceID)
		return err
	}
	// device name has been occupied
	if nil == err && deviceID != netDeviceID {
		blog.Errorf("[NetDevice] update net device fail, duplicate device name: [%s]", deviceInfo.DeviceName)
		return defErr.Errorf(common.CCErrCommDuplicateItem, "device")
	}

	return lgc.updateDevice(pheader, deviceInfo, netDeviceID, util.GetOwnerID(pheader))
}

// BatchCreateDevice bacth create or update net devices
func (lgc *Logics) BatchCreateDevice(pheader http.Header, deviceInfoList []meta.NetcollectDevice) ([]meta.BatchAddDeviceResult, bool) {
	ownerID := util.GetOwnerID(pheader)

	resultList := make([]meta.BatchAddDeviceResult, 0)
	hasError := false

	for _, deviceInfo := range deviceInfoList {
		errMsg := ""
		result := true

		deviceID, err := lgc.addOrUpdateDevice(pheader, deviceInfo, ownerID)
		if nil != err {
			errMsg = err.Error()
			result = false
			hasError = true
		}

		resultList = append(resultList,
			meta.BatchAddDeviceResult{Result: result, ErrMsg: errMsg, DeviceID: deviceID})
	}

	return resultList, hasError
}

// SearchDevice get net devices by conditions
func (lgc *Logics) SearchDevice(pheader http.Header, params *meta.NetCollSearchParams) (meta.SearchNetDevice, error) {
	defErr := lgc.Engine.CCErr.CreateDefaultCCErrorIf(util.GetLanguage(pheader))

	deviceCond := map[string]interface{}{}
	deviceCond[common.BKOwnerIDField] = util.GetOwnerID(pheader)

	objCond := map[string]interface{}{}

	// get condition, condtion of objs and condtion of device
	for _, cond := range params.Condition {
		switch cond.Operator {
		case common.BKDBEQ:
			if common.BKObjNameField == cond.Field {
				objCond[cond.Field] = cond.Value
			} else {
				deviceCond[cond.Field] = cond.Value
			}
		default:
			if common.BKObjNameField == cond.Field {
				objCond[cond.Field] = map[string]interface{}{
					cond.Operator: cond.Value,
				}
			} else {
				deviceCond[cond.Field] = map[string]interface{}{
					cond.Operator: cond.Value,
				}
			}
		}
	}

	// if condition only has bk_obj_name but not bk_obj_id
	// get net device bk_obj_id from bk_obj_name
	if _, ok := deviceCond[common.BKObjIDField]; !ok && 0 < len(objCond) {
		objIDs, err := lgc.getNetDeviceObjIDsByCond(pheader, objCond)
		if nil != err {
			blog.Errorf("[NetDevice] search net device fail, search net device obj id by condition [%#v] error: %v", objCond, err)
			return meta.SearchNetDevice{}, defErr.Error(common.CCErrCollectNetDeviceGetFail)
		}
		deviceCond[common.BKObjIDField] = map[string]interface{}{
			common.BKDBIN: objIDs,
		}
	}

	searchResult := meta.SearchNetDevice{}
	var err error

	searchResult.Count, err = lgc.db.Table(common.BKTableNameNetcollectDevice).Find(deviceCond).Count(lgc.ctx)
	if nil != err {
		blog.Errorf("[NetDevice] search net device fail, count net device by condition [%#v] error: %v", deviceCond, err)
		return meta.SearchNetDevice{}, err
	}
	if 0 == searchResult.Count {
		searchResult.Info = []meta.NetcollectDevice{}
		return searchResult, nil
	}

	// field bk_obj_id must be in params.Fields
	// to help add value of fields(bk_obj_name) from other tables into search result
	if 0 != len(params.Fields) {
		params.Fields = append(params.Fields, common.BKObjIDField)
	}
	if err = lgc.findDevice(params.Fields, deviceCond, &searchResult.Info, params.Page.Sort, params.Page.Start, params.Page.Limit); nil != err {
		blog.Errorf("[NetDevice] search net device fail, search net device by condition [%#v] error: %v", deviceCond, err)
		return meta.SearchNetDevice{}, defErr.Error(common.CCErrCollectNetDeviceGetFail)
	}

	objIDMapObjName, err := lgc.getObjIDMapObjNameFromNetDevice(pheader, searchResult.Info)
	if nil != err {
		return meta.SearchNetDevice{}, defErr.Error(common.CCErrCollectNetDeviceGetFail)
	}
	lgc.addShowFieldValueIntoNetDevice(searchResult.Info, objIDMapObjName)

	return searchResult, nil
}

func (lgc *Logics) DeleteDevice(pheader http.Header, netDeviceID uint64) error {
	defErr := lgc.Engine.CCErr.CreateDefaultCCErrorIf(util.GetLanguage(pheader))
	ownerID := util.GetOwnerID(pheader)

	deviceCond := map[string]interface{}{
		common.BKOwnerIDField:  ownerID,
		common.BKDeviceIDField: netDeviceID}

	// check if net device has property
	hasProperty, err := lgc.checkDeviceHasProperty(netDeviceID, ownerID)
	if nil != err {
		return defErr.Error(common.CCErrCollectNetDeviceDeleteFail)
	}
	if hasProperty {
		blog.Errorf("[NetDevice] delete net device fail, net device has property [%d]", netDeviceID)
		return defErr.Error(common.CCErrCollectNetDeviceHasPropertyDeleteFail)
	}

	if err = lgc.db.Table(common.BKTableNameNetcollectDevice).Delete(lgc.ctx, deviceCond); nil != err {
		blog.Errorf("[NetDevice] delete net device with id [%d] failed, err: %v, params: %#v", netDeviceID, err, deviceCond)
		return defErr.Error(common.CCErrCollectNetDeviceDeleteFail)
	}

	blog.V(5).Infof("[NetDevice] delete net device with id [%d] success", netDeviceID)
	return nil
}

func (lgc *Logics) addDevice(pheader http.Header, deviceInfo meta.NetcollectDevice, ownerID string) (uint64, error) {
	defErr := lgc.Engine.CCErr.CreateDefaultCCErrorIf(util.GetLanguage(pheader))

	isExist, err := lgc.checkNetDevice(pheader, &deviceInfo, ownerID)
	if nil != err {
		blog.Errorf("[NetDevice] add net device fail, %v", err)
		return INVALIDID, err
	}
	if isExist {
		blog.Errorf("[NetDevice] add net device fail, error: duplicate device_name")
		return INVALIDID, defErr.Errorf(common.CCErrCommDuplicateItem, "device")
	}

	// add to the storage
	deviceID, err := lgc.addNewDevice(deviceInfo, ownerID)
	if nil != err {
		blog.Errorf("[NetDevice] add net device fail, error: %v", err)
		return INVALIDID, defErr.Error(common.CCErrCollectNetDeviceCreateFail)
	}

	return deviceID, nil
}

func (lgc *Logics) updateDevice(
	pheader http.Header, deviceInfo meta.NetcollectDevice, netDeviceID uint64, ownerID string) error {

	defErr := lgc.Engine.CCErr.CreateDefaultCCErrorIf(util.GetLanguage(pheader))

	if _, err := lgc.checkNetDevice(pheader, &deviceInfo, ownerID); nil != err {
		blog.Errorf("[NetDevice] update net device fail, check device data error: %v", err)
		return err
	}

	// update to the storage
	deviceInfo.OwnerID = ownerID

	if err := lgc.updateExistingDeviceByDeviceID(deviceInfo, netDeviceID); nil != err {
		blog.Errorf("[NetDevice] update net device fail, update to database error: %v", err)
		return defErr.Error(common.CCErrCollectNetDeviceUpdateFail)
	}

	return nil
}

// add a device or update an existing device
func (lgc *Logics) addOrUpdateDevice(pheader http.Header, deviceInfo meta.NetcollectDevice, ownerID string) (uint64, error) {
	defErr := lgc.Engine.CCErr.CreateDefaultCCErrorIf(util.GetLanguage(pheader))

	isExist, err := lgc.checkNetDevice(pheader, &deviceInfo, ownerID)
	if nil != err {
		blog.Errorf("[NetDevice] batch add net device fail, %v", err)
		return INVALIDID, err
	}
	if isExist {
		// get updated device ID
		deviceID, _, err := lgc.checkNetDeviceExist(pheader, 0, deviceInfo.DeviceName)
		if nil != err {
			blog.Errorf("[NetDevice] batch add net device, get updated device id fail, error: %v", err)
			return INVALIDID, defErr.Errorf(common.CCErrCollectNetDeviceUpdateFail)
		}

		// update to the storage
		deviceInfo.OwnerID = ownerID

		if err := lgc.updateExistingDeviceByDeviceName(deviceInfo); nil != err {
			blog.Errorf("[NetDevice] batch add net device fail, error: %v", err)
			return INVALIDID, defErr.Error(common.CCErrCollectNetDeviceUpdateFail)
		}

		return deviceID, nil
	}

	// add to the storage
	deviceID, err := lgc.addNewDevice(deviceInfo, ownerID)
	if nil != err {
		blog.Errorf("[NetDevice] batch add net device fail, error: %v", err)
		return INVALIDID, defErr.Error(common.CCErrCollectNetDeviceCreateFail)
	}

	return deviceID, nil
}

func (lgc *Logics) findDevice(fields []string, condition, result interface{}, sort string, skip, limit int) error {
	if err := lgc.db.Table(common.BKTableNameNetcollectDevice).Find(condition).Fields(fields...).Sort(sort).Start(uint64(skip)).Limit(uint64(limit)).All(lgc.ctx, result); err != nil {
		blog.Errorf("[NetDevice] failed to query the device, condition: %#+v, error: %s", condition, err.Error())
		return err
	}

	return nil
}

func (lgc *Logics) checkNetDevice(
	pheader http.Header, deviceInfo *meta.NetcollectDevice, ownerID string) (isExist bool, err error) {

	defErr := lgc.Engine.CCErr.CreateDefaultCCErrorIf(util.GetLanguage(pheader))

	if "" == deviceInfo.DeviceModel {
		return false, defErr.Errorf(common.CCErrCommParamsNeedSet, common.BKDeviceModelField)
	}

	if "" == deviceInfo.BkVendor {
		return false, defErr.Errorf(common.CCErrCommParamsNeedSet, common.BKVendorField)
	}

	if "" == deviceInfo.DeviceName {
		return false, defErr.Errorf(common.CCErrCommParamsNeedSet, common.BKDeviceNameField)
	}

	// check if bk_object_id and bk_object_name are net device object
	if err := lgc.checkIfNetDeviceObject(pheader, deviceInfo); nil != err {
		blog.Errorf("[NetDevice] check net device fail, object name [%s] and object ID [%s] is not netcollect device",
			deviceInfo.ObjectName, deviceInfo.ObjectID)
		return false, err
	}

	// check if device_name exist
	isExist, err = lgc.checkIfNetDeviceNameExist(deviceInfo.DeviceName, ownerID)
	if nil != err {
		return false, defErr.Error(common.CCErrCommDBSelectFailed)
	}

	return isExist, nil
}

func (lgc *Logics) addNewDevice(deviceInfo meta.NetcollectDevice, ownerID string) (deviceID uint64, err error) {
	now := util.GetCurrentTimePtr()
	deviceInfo.CreateTime = now
	deviceInfo.LastTime = now
	deviceInfo.OwnerID = ownerID

	deviceInfo.DeviceID, err = lgc.db.NextSequence(lgc.ctx, common.BKTableNameNetcollectDevice)
	if nil != err {
		return INVALIDID, fmt.Errorf("failed to get id, %v", err)
	}

	if err = lgc.db.Table(common.BKTableNameNetcollectDevice).Insert(lgc.ctx, deviceInfo); nil != err {
		return INVALIDID, err
	}

	blog.V(5).Infof("[NetDevice] add net device, deviceInfo [%#+v]", deviceInfo)

	return deviceInfo.DeviceID, nil
}

func (lgc *Logics) updateExistingDeviceByDeviceID(deviceInfo meta.NetcollectDevice, netDeviceID uint64) error {
	queryParams := mapstr.MapStr{
		common.BKDeviceIDField: netDeviceID,
		common.BKOwnerIDField:  deviceInfo.OwnerID,
	}

	deviceInfo.LastTime = util.GetCurrentTimePtr()
	deviceInfo.DeviceID = netDeviceID

	if err := lgc.db.Table(common.BKTableNameNetcollectDevice).Update(lgc.ctx, queryParams, deviceInfo); nil != err {
		blog.Errorf("[NetDevice] update net device by id fail, error: %v, queryParams: [%#+v], deviceInfo: [%#+v]",
			err, queryParams, deviceInfo)
		return err
	}

	blog.V(5).Infof("[NetDevice] update net device by id [%d] deviceInfo [%#+v]", netDeviceID, deviceInfo)

	return nil
}

func (lgc *Logics) updateExistingDeviceByDeviceName(deviceInfo meta.NetcollectDevice) error {
	queryParams := mapstr.MapStr{
		common.BKDeviceNameField: deviceInfo.DeviceName,
		common.BKOwnerIDField:    deviceInfo.OwnerID,
	}

	deviceInfo.LastTime = util.GetCurrentTimePtr()

	if err := lgc.db.Table(common.BKTableNameNetcollectDevice).Update(lgc.ctx, queryParams, deviceInfo); nil != err {
		blog.Errorf("[NetDevice] update net device by name fail, error: %v, queryParams: [%#+v], deviceInfo: [%#+v]",
			err, queryParams, deviceInfo)
		return err
	}

	blog.V(5).Infof("[NetDevice] update net device by name [%s], deviceInfo [%#+v]", deviceInfo.DeviceName, deviceInfo)

	return nil
}

// get objID map objName from objID of net device
func (lgc *Logics) getObjIDMapObjNameFromNetDevice(
	pheader http.Header, netDevice []meta.NetcollectDevice) (map[string]string, error) {

	defErr := lgc.Engine.CCErr.CreateDefaultCCErrorIf(util.GetLanguage(pheader))

	objIDs := make([]string, 0)
	for index := range netDevice {
		objIDs = append(objIDs, netDevice[index].ObjectID)
	}

	objCond := map[string]interface{}{
		common.BKClassificationIDField: common.BKNetwork,
	}
	if 0 != len(objIDs) {
		objCond[common.BKObjIDField] = map[string]interface{}{
			common.BKDBIN: objIDs,
		}
	}

	objResult, err := lgc.CoreAPI.CoreService().Model().ReadModel(context.Background(), pheader, &meta.QueryCondition{Condition: objCond})
	if nil != err {
		blog.Errorf("[NetDevice] search net device object, search objectName fail, %v", err)
		return nil, err
	}
	if !objResult.Result {
		blog.Errorf("[NetDevice] search net device object, errors: %s", objResult.ErrMsg)
		return nil, defErr.New(objResult.Code, objResult.ErrMsg)
	}

	objIDMapObjName := map[string]string{}
	for _, data := range objResult.Data.Info {
		objIDMapObjName[data.Spec.ObjectID] = data.Spec.ObjectName
	}

	return objIDMapObjName, nil
}

func (lgc *Logics) addShowFieldValueIntoNetDevice(
	netDevice []meta.NetcollectDevice, objIDMapObjName map[string]string) {

	for index := range netDevice {
		objName := objIDMapObjName[netDevice[index].ObjectID]
		netDevice[index].ObjectName = objName
	}
}

// check the deviceInfo if is a net object
// by checking if bk_obj_id and bk_obj_name function parameter are valid net device object or not
func (lgc *Logics) checkIfNetDeviceObject(pheader http.Header, deviceInfo *meta.NetcollectDevice) error {
	objectID, objectName, err := lgc.checkNetObject(pheader, deviceInfo.ObjectID, deviceInfo.ObjectName)
	if nil != err {
		return err
	}
	deviceInfo.ObjectID, deviceInfo.ObjectName = objectID, objectName
	return nil
}

// check if net device name exist
func (lgc *Logics) checkIfNetDeviceNameExist(deviceName string, ownerID string) (bool, error) {
	queryParams := mapstr.MapStr{common.BKDeviceNameField: deviceName, common.BKOwnerIDField: ownerID}

	rowCount, err := lgc.db.Table(common.BKTableNameNetcollectDevice).Find(queryParams).Count(lgc.ctx)
	if nil != err {
		blog.Errorf("[NetDevice] check if net device name exist, query device fail, error information is %v, params:%v",
			err, queryParams)
		return false, err
	}

	if 0 != rowCount {
		blog.V(5).Infof("[NetDevice] check if net device name exist, bk_device_name is [%s] device is exist", deviceName)
		return true, nil
	}

	return false, nil
}

// check if net device name exist
func (lgc *Logics) getNetDeviceIDByName(deviceName string, ownerID string) (uint64, error) {
	queryParams := mapstr.MapStr{common.BKDeviceNameField: deviceName, common.BKOwnerIDField: ownerID}

	result := meta.NetcollectDevice{}

	if err := lgc.db.Table(common.BKTableNameNetcollectDevice).Find(queryParams).All(lgc.ctx, &result); nil != err {
		blog.Errorf("[NetDevice] get net device ID by name, query device fail, error information is %v, params:%v",
			err, queryParams)
		return 0, err
	}

	return result.DeviceID, nil
}

// get net device obj ID
func (lgc *Logics) getNetDeviceObjIDsByCond(pheader http.Header, objCond map[string]interface{}) ([]string, error) {
	defErr := lgc.Engine.CCErr.CreateDefaultCCErrorIf(util.GetLanguage(pheader))

	objIDs := make([]string, 0)

	if _, ok := objCond[common.BKObjNameField]; ok {
		objCond[common.BKClassificationIDField] = common.BKNetwork
		objResult, err := lgc.CoreAPI.CoreService().Model().ReadModel(context.Background(), pheader, &meta.QueryCondition{Condition: objCond})
		if nil != err {
			blog.Errorf("[NetDevice] check net device object ID, search objectName fail, %v", err)
			return nil, err
		}

		if !objResult.Result {
			blog.Errorf("[NetDevice] check net device object ID, errors: %s", objResult.ErrMsg)
			return nil, defErr.New(objResult.Code, objResult.ErrMsg)
		}

		for _, data := range objResult.Data.Info {
			objIDs = append(objIDs, data.Spec.ObjectID)
		}
	}

	return objIDs, nil
}

// check if device has property
func (lgc *Logics) checkDeviceHasProperty(deviceID uint64, ownerID string) (bool, error) {
	queryParams := mapstr.MapStr{
		common.BKDeviceIDField: deviceID, common.BKOwnerIDField: ownerID}

	rowCount, err := lgc.db.Table(common.BKTableNameNetcollectProperty).Find(queryParams).Count(lgc.ctx)
	if nil != err {
		blog.Errorf("[NetDevice] check if net deviceID and propertyID exist, query device fail, error information is %v, params:%v",
			err, queryParams)
		return false, err
	}

	return 0 != rowCount, nil
}
