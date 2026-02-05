package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPhoneNumberTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{}, &Group{}, &SipUser{}, &PhoneNumber{})
	require.NoError(t, err)

	return db
}

func createTestUserForPhoneNumber(t *testing.T, db *gorm.DB) *User {
	user := &User{
		Email:    "phone@example.com",
		Password: "hashedpassword",
	}
	err := db.Create(user).Error
	require.NoError(t, err)
	return user
}

func createTestSipUser(t *testing.T, db *gorm.DB, userID uint) *SipUser {
	sipUser := &SipUser{
		UserID:     &userID,
		SchemeName: "Test Scheme",
		Status:     SipUserStatusRegistered,
	}
	err := db.Create(sipUser).Error
	require.NoError(t, err)
	return sipUser
}

func TestPhoneNumber_CRUD(t *testing.T) {
	db := setupPhoneNumberTestDB(t)
	user := createTestUserForPhoneNumber(t, db)
	sipUser := createTestSipUser(t, db, user.ID)

	// 测试创建号码
	phoneNumber := &PhoneNumber{
		UserID:             user.ID,
		PhoneNumber:        "13800138000",
		CountryCode:        "+86",
		Carrier:            "中国移动",
		Location:           "北京市",
		Alias:              "工作手机",
		Description:        "主要用于工作联系",
		Status:             PhoneNumberStatusActive,
		IsVerified:         true,
		IsPrimary:          true,
		ActiveSchemeID:     &sipUser.ID,
		CallForwardEnabled: true,
		CallForwardStatus:  CallForwardStatusEnabled,
		CallForwardNumber:  "13900139000",
	}

	err := CreatePhoneNumber(db, phoneNumber)
	assert.NoError(t, err)
	assert.NotZero(t, phoneNumber.ID)
	assert.NotZero(t, phoneNumber.CreatedAt)

	// 测试读取号码
	retrieved, err := GetPhoneNumberByID(db, phoneNumber.ID)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "13800138000", retrieved.PhoneNumber)
	assert.Equal(t, "+86", retrieved.CountryCode)
	assert.Equal(t, "中国移动", retrieved.Carrier)
	assert.Equal(t, "北京市", retrieved.Location)
	assert.Equal(t, "工作手机", retrieved.Alias)
	assert.Equal(t, PhoneNumberStatusActive, retrieved.Status)
	assert.True(t, retrieved.IsVerified)
	assert.True(t, retrieved.IsPrimary)
	assert.True(t, retrieved.CallForwardEnabled)
	assert.Equal(t, CallForwardStatusEnabled, retrieved.CallForwardStatus)
	assert.Equal(t, "13900139000", retrieved.CallForwardNumber)
	assert.Equal(t, user.Email, retrieved.User.Email)
	assert.Equal(t, sipUser.Username, retrieved.ActiveScheme.Username)

	// 测试更新号码
	retrieved.Alias = "更新后的别名"
	retrieved.Description = "更新后的描述"
	retrieved.CallForwardEnabled = false
	retrieved.CallForwardStatus = CallForwardStatusDisabled
	err = UpdatePhoneNumber(db, retrieved)
	assert.NoError(t, err)

	updated, err := GetPhoneNumberByID(db, phoneNumber.ID)
	assert.NoError(t, err)
	assert.Equal(t, "更新后的别名", updated.Alias)
	assert.Equal(t, "更新后的描述", updated.Description)
	assert.False(t, updated.CallForwardEnabled)
	assert.Equal(t, CallForwardStatusDisabled, updated.CallForwardStatus)

	// 测试软删除
	err = DeletePhoneNumber(db, phoneNumber.ID)
	assert.NoError(t, err)

	deleted, err := GetPhoneNumberByID(db, phoneNumber.ID)
	assert.Error(t, err)
	assert.Nil(t, deleted)
}

func TestGetPhoneNumberByNumber(t *testing.T) {
	db := setupPhoneNumberTestDB(t)
	user := createTestUserForPhoneNumber(t, db)

	phoneNumber := &PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13800138001",
		Status:      PhoneNumberStatusActive,
	}
	err := CreatePhoneNumber(db, phoneNumber)
	require.NoError(t, err)

	// 测试根据号码查找
	found, err := GetPhoneNumberByNumber(db, user.ID, "13800138001")
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, phoneNumber.ID, found.ID)
	assert.Equal(t, "13800138001", found.PhoneNumber)

	// 测试查找不存在的号码
	notFound, err := GetPhoneNumberByNumber(db, user.ID, "13800138999")
	assert.Error(t, err)
	assert.Nil(t, notFound)

	// 测试其他用户的号码
	otherUser := &User{
		Email:    "other@example.com",
		Password: "hashedpassword",
	}
	err = db.Create(otherUser).Error
	require.NoError(t, err)

	notFound, err = GetPhoneNumberByNumber(db, otherUser.ID, "13800138001")
	assert.Error(t, err)
	assert.Nil(t, notFound)
}

func TestGetPhoneNumbersByUserID(t *testing.T) {
	db := setupPhoneNumberTestDB(t)
	user := createTestUserForPhoneNumber(t, db)

	// 创建多个号码
	phoneNumbers := []*PhoneNumber{
		{
			UserID:      user.ID,
			PhoneNumber: "13800138001",
			IsPrimary:   true,
			Status:      PhoneNumberStatusActive,
		},
		{
			UserID:      user.ID,
			PhoneNumber: "13800138002",
			IsPrimary:   false,
			Status:      PhoneNumberStatusActive,
		},
		{
			UserID:      user.ID,
			PhoneNumber: "13800138003",
			IsPrimary:   false,
			Status:      PhoneNumberStatusInactive,
		},
	}

	for _, pn := range phoneNumbers {
		err := CreatePhoneNumber(db, pn)
		require.NoError(t, err)
	}

	// 获取用户的所有号码
	userPhoneNumbers, err := GetPhoneNumbersByUserID(db, user.ID)
	assert.NoError(t, err)
	assert.Len(t, userPhoneNumbers, 3)

	// 验证排序（主号码在前，然后按创建时间倒序）
	assert.True(t, userPhoneNumbers[0].IsPrimary)
	assert.Equal(t, "13800138001", userPhoneNumbers[0].PhoneNumber)
}

func TestGetPrimaryPhoneNumber(t *testing.T) {
	db := setupPhoneNumberTestDB(t)
	user := createTestUserForPhoneNumber(t, db)

	// 创建非主号码
	phoneNumber1 := &PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13800138001",
		IsPrimary:   false,
		Status:      PhoneNumberStatusActive,
	}
	err := CreatePhoneNumber(db, phoneNumber1)
	require.NoError(t, err)

	// 创建主号码
	phoneNumber2 := &PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13800138002",
		IsPrimary:   true,
		Status:      PhoneNumberStatusActive,
	}
	err = CreatePhoneNumber(db, phoneNumber2)
	require.NoError(t, err)

	// 获取主号码
	primary, err := GetPrimaryPhoneNumber(db, user.ID)
	assert.NoError(t, err)
	assert.NotNil(t, primary)
	assert.Equal(t, phoneNumber2.ID, primary.ID)
	assert.Equal(t, "13800138002", primary.PhoneNumber)
	assert.True(t, primary.IsPrimary)

	// 测试没有主号码的情况
	otherUser := &User{
		Email:    "other@example.com",
		Password: "hashedpassword",
	}
	err = db.Create(otherUser).Error
	require.NoError(t, err)

	noPrimary, err := GetPrimaryPhoneNumber(db, otherUser.ID)
	assert.Error(t, err)
	assert.Nil(t, noPrimary)
}

func TestSetPrimaryPhoneNumber(t *testing.T) {
	db := setupPhoneNumberTestDB(t)
	user := createTestUserForPhoneNumber(t, db)

	// 创建多个号码
	phoneNumber1 := &PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13800138001",
		IsPrimary:   true,
		Status:      PhoneNumberStatusActive,
	}
	err := CreatePhoneNumber(db, phoneNumber1)
	require.NoError(t, err)

	phoneNumber2 := &PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13800138002",
		IsPrimary:   false,
		Status:      PhoneNumberStatusActive,
	}
	err = CreatePhoneNumber(db, phoneNumber2)
	require.NoError(t, err)

	// 设置新的主号码
	err = SetPrimaryPhoneNumber(db, user.ID, phoneNumber2.ID)
	assert.NoError(t, err)

	// 验证主号码已更改
	primary, err := GetPrimaryPhoneNumber(db, user.ID)
	assert.NoError(t, err)
	assert.Equal(t, phoneNumber2.ID, primary.ID)
	assert.True(t, primary.IsPrimary)

	// 验证原主号码已取消
	updated1, err := GetPhoneNumberByID(db, phoneNumber1.ID)
	assert.NoError(t, err)
	assert.False(t, updated1.IsPrimary)
}

func TestBindSchemeToPhoneNumber(t *testing.T) {
	db := setupPhoneNumberTestDB(t)
	user := createTestUserForPhoneNumber(t, db)
	sipUser := createTestSipUser(t, db, user.ID)

	phoneNumber := &PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13800138001",
		Status:      PhoneNumberStatusActive,
	}
	err := CreatePhoneNumber(db, phoneNumber)
	require.NoError(t, err)

	// 绑定代接方案
	err = BindSchemeToPhoneNumber(db, phoneNumber.ID, sipUser.ID)
	assert.NoError(t, err)

	// 验证绑定
	updated, err := GetPhoneNumberByID(db, phoneNumber.ID)
	assert.NoError(t, err)
	assert.NotNil(t, updated.ActiveSchemeID)
	assert.Equal(t, sipUser.ID, *updated.ActiveSchemeID)
	assert.Equal(t, sipUser.Username, updated.ActiveScheme.Username)
}

func TestUnbindSchemeFromPhoneNumber(t *testing.T) {
	db := setupPhoneNumberTestDB(t)
	user := createTestUserForPhoneNumber(t, db)
	sipUser := createTestSipUser(t, db, user.ID)

	phoneNumber := &PhoneNumber{
		UserID:         user.ID,
		PhoneNumber:    "13800138001",
		Status:         PhoneNumberStatusActive,
		ActiveSchemeID: &sipUser.ID,
	}
	err := CreatePhoneNumber(db, phoneNumber)
	require.NoError(t, err)

	// 解绑代接方案
	err = UnbindSchemeFromPhoneNumber(db, phoneNumber.ID)
	assert.NoError(t, err)

	// 验证解绑
	updated, err := GetPhoneNumberByID(db, phoneNumber.ID)
	assert.NoError(t, err)
	assert.Nil(t, updated.ActiveSchemeID)
}

func TestUpdateCallForwardStatus(t *testing.T) {
	db := setupPhoneNumberTestDB(t)
	user := createTestUserForPhoneNumber(t, db)

	phoneNumber := &PhoneNumber{
		UserID:             user.ID,
		PhoneNumber:        "13800138001",
		Status:             PhoneNumberStatusActive,
		CallForwardEnabled: false,
		CallForwardStatus:  CallForwardStatusDisabled,
	}
	err := CreatePhoneNumber(db, phoneNumber)
	require.NoError(t, err)

	// 启用呼叫转移
	err = UpdateCallForwardStatus(db, phoneNumber.ID, true, CallForwardStatusEnabled)
	assert.NoError(t, err)

	// 验证更新
	updated, err := GetPhoneNumberByID(db, phoneNumber.ID)
	assert.NoError(t, err)
	assert.True(t, updated.CallForwardEnabled)
	assert.Equal(t, CallForwardStatusEnabled, updated.CallForwardStatus)
	assert.NotNil(t, updated.CallForwardSetAt)

	// 禁用呼叫转移
	err = UpdateCallForwardStatus(db, phoneNumber.ID, false, CallForwardStatusDisabled)
	assert.NoError(t, err)

	updated, err = GetPhoneNumberByID(db, phoneNumber.ID)
	assert.NoError(t, err)
	assert.False(t, updated.CallForwardEnabled)
	assert.Equal(t, CallForwardStatusDisabled, updated.CallForwardStatus)
}

func TestPhoneNumber_StatusTransitions(t *testing.T) {
	db := setupPhoneNumberTestDB(t)
	user := createTestUserForPhoneNumber(t, db)

	phoneNumber := &PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13800138001",
		Status:      PhoneNumberStatusInactive,
		IsVerified:  false,
	}
	err := CreatePhoneNumber(db, phoneNumber)
	require.NoError(t, err)

	// 状态转换：未激活 -> 验证中
	phoneNumber.Status = PhoneNumberStatusVerifying
	err = UpdatePhoneNumber(db, phoneNumber)
	assert.NoError(t, err)

	// 状态转换：验证中 -> 激活
	now := time.Now()
	phoneNumber.Status = PhoneNumberStatusActive
	phoneNumber.IsVerified = true
	phoneNumber.VerifiedAt = &now
	err = UpdatePhoneNumber(db, phoneNumber)
	assert.NoError(t, err)

	// 验证状态更新
	updated, err := GetPhoneNumberByID(db, phoneNumber.ID)
	assert.NoError(t, err)
	assert.Equal(t, PhoneNumberStatusActive, updated.Status)
	assert.True(t, updated.IsVerified)
	assert.NotNil(t, updated.VerifiedAt)

	// 状态转换：激活 -> 暂停
	updated.Status = PhoneNumberStatusSuspended
	err = UpdatePhoneNumber(db, updated)
	assert.NoError(t, err)

	suspended, err := GetPhoneNumberByID(db, phoneNumber.ID)
	assert.NoError(t, err)
	assert.Equal(t, PhoneNumberStatusSuspended, suspended.Status)
}

func TestPhoneNumber_CallForwardStatuses(t *testing.T) {
	db := setupPhoneNumberTestDB(t)
	user := createTestUserForPhoneNumber(t, db)

	statuses := []CallForwardStatus{
		CallForwardStatusEnabled,
		CallForwardStatusDisabled,
		CallForwardStatusUnknown,
	}

	for i, status := range statuses {
		phoneNumber := &PhoneNumber{
			UserID:            user.ID,
			PhoneNumber:       "1380013800" + string(rune('1'+i)),
			Status:            PhoneNumberStatusActive,
			CallForwardStatus: status,
		}
		err := CreatePhoneNumber(db, phoneNumber)
		require.NoError(t, err)

		retrieved, err := GetPhoneNumberByID(db, phoneNumber.ID)
		assert.NoError(t, err)
		assert.Equal(t, status, retrieved.CallForwardStatus)
	}
}

func TestPhoneNumber_WithGroup(t *testing.T) {
	db := setupPhoneNumberTestDB(t)
	user := createTestUserForPhoneNumber(t, db)

	// 创建组织
	group := &Group{
		Name:      "Test Company",
		Type:      "company",
		CreatorID: user.ID,
	}
	err := db.Create(group).Error
	require.NoError(t, err)

	// 创建组织号码
	phoneNumber := &PhoneNumber{
		UserID:      user.ID,
		GroupID:     &group.ID,
		PhoneNumber: "13800138001",
		Status:      PhoneNumberStatusActive,
		Alias:       "公司总机",
	}
	err = CreatePhoneNumber(db, phoneNumber)
	require.NoError(t, err)

	// 验证组织关联
	retrieved, err := GetPhoneNumberByID(db, phoneNumber.ID)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved.GroupID)
	assert.Equal(t, group.ID, *retrieved.GroupID)
	assert.Equal(t, group.Name, retrieved.Group.Name)
}

func TestPhoneNumber_Statistics(t *testing.T) {
	db := setupPhoneNumberTestDB(t)
	user := createTestUserForPhoneNumber(t, db)

	phoneNumber := &PhoneNumber{
		UserID:          user.ID,
		PhoneNumber:     "13800138001",
		Status:          PhoneNumberStatusActive,
		TotalCalls:      100,
		TotalVoicemails: 25,
	}
	err := CreatePhoneNumber(db, phoneNumber)
	require.NoError(t, err)

	// 更新统计信息
	now := time.Now()
	phoneNumber.TotalCalls = 150
	phoneNumber.TotalVoicemails = 30
	phoneNumber.LastCallAt = &now
	err = UpdatePhoneNumber(db, phoneNumber)
	assert.NoError(t, err)

	// 验证统计信息
	updated, err := GetPhoneNumberByID(db, phoneNumber.ID)
	assert.NoError(t, err)
	assert.Equal(t, 150, updated.TotalCalls)
	assert.Equal(t, 30, updated.TotalVoicemails)
	assert.NotNil(t, updated.LastCallAt)
}

func TestPhoneNumber_Metadata(t *testing.T) {
	db := setupPhoneNumberTestDB(t)
	user := createTestUserForPhoneNumber(t, db)

	metadata := `{"region": "北京", "operator": "移动", "plan": "无限流量套餐"}`
	notes := "这是一个测试号码，用于开发环境"

	phoneNumber := &PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13800138001",
		Status:      PhoneNumberStatusActive,
		Metadata:    metadata,
		Notes:       notes,
	}
	err := CreatePhoneNumber(db, phoneNumber)
	require.NoError(t, err)

	// 验证元数据存储
	retrieved, err := GetPhoneNumberByID(db, phoneNumber.ID)
	assert.NoError(t, err)
	assert.Equal(t, metadata, retrieved.Metadata)
	assert.Equal(t, notes, retrieved.Notes)
}

// Benchmark tests
func BenchmarkCreatePhoneNumber(b *testing.B) {
	db := setupPhoneNumberTestDB(&testing.T{})
	user := createTestUserForPhoneNumber(&testing.T{}, db)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		phoneNumber := &PhoneNumber{
			UserID:      user.ID,
			PhoneNumber: "1380013800" + string(rune('0'+i%10)),
			Status:      PhoneNumberStatusActive,
		}
		CreatePhoneNumber(db, phoneNumber)
	}
}

func BenchmarkGetPhoneNumberByNumber(b *testing.B) {
	db := setupPhoneNumberTestDB(&testing.T{})
	user := createTestUserForPhoneNumber(&testing.T{}, db)

	phoneNumber := &PhoneNumber{
		UserID:      user.ID,
		PhoneNumber: "13800138000",
		Status:      PhoneNumberStatusActive,
	}
	CreatePhoneNumber(db, phoneNumber)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetPhoneNumberByNumber(db, user.ID, "13800138000")
	}
}
