package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSipUserTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{}, &Group{}, &Assistant{}, &SipUser{})
	require.NoError(t, err)

	return db
}

func createTestUserForSipUser(t *testing.T, db *gorm.DB) *User {
	user := &User{
		Email:    "sipuser@example.com",
		Password: "hashedpassword",
	}
	err := db.Create(user).Error
	require.NoError(t, err)
	return user
}

func createTestAssistantForSipUser(t *testing.T, db *gorm.DB, userID uint) *Assistant {
	assistant := &Assistant{
		UserID:      userID,
		Name:        "SIP Assistant",
		Description: "Assistant for SIP calls",
	}
	err := db.Create(assistant).Error
	require.NoError(t, err)
	return assistant
}

func TestKeywordReplies_Value(t *testing.T) {
	kr := KeywordReplies{
		KeywordReply{Keyword: "hello", Reply: "Hi there!"},
		KeywordReply{Keyword: "help", Reply: "How can I assist you?"},
	}

	value, err := kr.Value()
	assert.NoError(t, err)
	assert.NotNil(t, value)

	// 测试空切片
	emptyKr := KeywordReplies{}
	value, err = emptyKr.Value()
	assert.NoError(t, err)
	assert.Nil(t, value)

	// 测试nil切片
	var nilKr KeywordReplies
	value, err = nilKr.Value()
	assert.NoError(t, err)
	assert.Nil(t, value)
}

func TestKeywordReplies_Scan(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected KeywordReplies
		wantErr  bool
	}{
		{
			name:     "valid JSON bytes",
			input:    []byte(`[{"keyword":"hello","reply":"Hi!"}]`),
			expected: KeywordReplies{{Keyword: "hello", Reply: "Hi!"}},
			wantErr:  false,
		},
		{
			name:     "nil input",
			input:    nil,
			expected: KeywordReplies{},
			wantErr:  false,
		},
		{
			name:     "empty bytes",
			input:    []byte{},
			expected: KeywordReplies{},
			wantErr:  false,
		},
		{
			name:    "invalid type",
			input:   "invalid",
			wantErr: false, // 函数返回nil而不是错误
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var kr KeywordReplies
			err := kr.Scan(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.input != "invalid" {
					assert.Equal(t, tt.expected, kr)
				}
			}
		})
	}
}

func TestSipUser_CRUD(t *testing.T) {
	db := setupSipUserTestDB(t)
	user := createTestUserForSipUser(t, db)
	assistant := createTestAssistantForSipUser(t, db, user.ID)

	// 测试创建SIP用户
	sipUser := &SipUser{
		SchemeName:      "工作模式",
		Description:     "工作时间的代接方案",
		Username:        "work-scheme-001",
		Password:        "secure-password",
		Contact:         "sip:work-scheme-001@example.com",
		ContactIP:       "192.168.1.100",
		ContactPort:     5060,
		Expires:         3600,
		Status:          SipUserStatusRegistered,
		UserAgent:       "LingEcho SIP Client 1.0",
		RemoteIP:        "203.0.113.10",
		UserID:          &user.ID,
		AssistantID:     func() *uint { id := uint(assistant.ID); return &id }(),
		AutoAnswer:      true,
		AutoAnswerDelay: 3,
		OpeningMessage:  "您好，我是智能助手，请问有什么可以帮助您的？",
		KeywordReplies: KeywordReplies{
			{Keyword: "价格", Reply: "关于价格信息，请稍等，我为您查询。"},
			{Keyword: "地址", Reply: "我们的地址是北京市朝阳区xxx路xxx号。"},
		},
		FallbackMessage:  "抱歉，我没有理解您的问题，请您再详细说明一下。",
		AIFreeResponse:   true,
		RecordingEnabled: true,
		RecordingMode:    RecordingModeFull,
		RecordingPath:    "/recordings/{date}/{callid}.wav",
		MessageEnabled:   true,
		MessageDuration:  30,
		MessagePrompt:    "请在嘀声后留言，谢谢。",
		BoundPhoneNumber: "13800138000",
		DisplayName:      "工作助手",
		Alias:            "Work Assistant",
		Enabled:          true,
		IsActive:         true,
		Notes:            "主要用于工作时间的来电处理",
	}

	err := CreateSipUser(db, sipUser)
	assert.NoError(t, err)
	assert.NotZero(t, sipUser.ID)
	assert.NotZero(t, sipUser.CreatedAt)

	// 测试读取SIP用户
	retrieved, err := GetSipUserByUsername(db, "work-scheme-001")
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, "工作模式", retrieved.SchemeName)
	assert.Equal(t, "work-scheme-001", retrieved.Username)
	assert.Equal(t, SipUserStatusRegistered, retrieved.Status)
	assert.Equal(t, "sip:work-scheme-001@example.com", retrieved.Contact)
	assert.Equal(t, "192.168.1.100", retrieved.ContactIP)
	assert.Equal(t, 5060, retrieved.ContactPort)
	assert.Equal(t, 3600, retrieved.Expires)
	assert.Equal(t, user.ID, *retrieved.UserID)
	assert.Equal(t, uint(assistant.ID), *retrieved.AssistantID)
	assert.True(t, retrieved.AutoAnswer)
	assert.Equal(t, 3, retrieved.AutoAnswerDelay)
	assert.Equal(t, "您好，我是智能助手，请问有什么可以帮助您的？", retrieved.OpeningMessage)
	assert.Len(t, retrieved.KeywordReplies, 2)
	assert.Equal(t, "价格", retrieved.KeywordReplies[0].Keyword)
	assert.Equal(t, "关于价格信息，请稍等，我为您查询。", retrieved.KeywordReplies[0].Reply)
	assert.True(t, retrieved.AIFreeResponse)
	assert.True(t, retrieved.RecordingEnabled)
	assert.Equal(t, RecordingModeFull, retrieved.RecordingMode)
	assert.True(t, retrieved.MessageEnabled)
	assert.Equal(t, 30, retrieved.MessageDuration)
	assert.Equal(t, "13800138000", retrieved.BoundPhoneNumber)
	assert.True(t, retrieved.Enabled)
	assert.True(t, retrieved.IsActive)

	// 测试更新SIP用户
	retrieved.SchemeName = "更新后的工作模式"
	retrieved.AutoAnswerDelay = 5
	retrieved.MessageDuration = 60
	retrieved.RegisterCount = 10
	retrieved.CallCount = 25
	retrieved.TotalCallDuration = 3600
	retrieved.MessageCount = 5

	err = UpdateSipUser(db, retrieved)
	assert.NoError(t, err)

	updated, err := GetSipUserByID(db, retrieved.ID)
	assert.NoError(t, err)
	assert.Equal(t, "更新后的工作模式", updated.SchemeName)
	assert.Equal(t, 5, updated.AutoAnswerDelay)
	assert.Equal(t, 60, updated.MessageDuration)
	assert.Equal(t, 10, updated.RegisterCount)
	assert.Equal(t, 25, updated.CallCount)
	assert.Equal(t, 3600, updated.TotalCallDuration)
	assert.Equal(t, 5, updated.MessageCount)

	// 测试软删除
	err = DeleteSipUser(db, retrieved.ID)
	assert.NoError(t, err)

	deleted, err := GetSipUserByID(db, retrieved.ID)
	assert.Error(t, err)
	assert.Nil(t, deleted)
}

func TestSipUser_IsRegistered(t *testing.T) {
	sipUser := &SipUser{Status: SipUserStatusRegistered}
	assert.True(t, sipUser.IsRegistered())

	sipUser.Status = SipUserStatusUnregistered
	assert.False(t, sipUser.IsRegistered())

	sipUser.Status = SipUserStatusExpired
	assert.False(t, sipUser.IsRegistered())
}

func TestSipUser_IsExpired(t *testing.T) {
	sipUser := &SipUser{}

	// 测试没有过期时间的情况
	assert.False(t, sipUser.IsExpired())

	// 测试未过期的情况
	futureTime := time.Now().Add(time.Hour)
	sipUser.ExpiresAt = &futureTime
	assert.False(t, sipUser.IsExpired())

	// 测试已过期的情况
	pastTime := time.Now().Add(-time.Hour)
	sipUser.ExpiresAt = &pastTime
	assert.True(t, sipUser.IsExpired())
}

func TestSipUser_UpdateExpiresAt(t *testing.T) {
	sipUser := &SipUser{Expires: 3600}

	beforeUpdate := time.Now()
	sipUser.UpdateExpiresAt()
	afterUpdate := time.Now()

	assert.NotNil(t, sipUser.ExpiresAt)
	assert.True(t, sipUser.ExpiresAt.After(beforeUpdate.Add(3500*time.Second)))
	assert.True(t, sipUser.ExpiresAt.Before(afterUpdate.Add(3700*time.Second)))

	// 测试Expires为0的情况
	sipUser.Expires = 0
	sipUser.ExpiresAt = nil
	sipUser.UpdateExpiresAt()
	assert.Nil(t, sipUser.ExpiresAt)
}

func TestGetRegisteredSipUsers(t *testing.T) {
	db := setupSipUserTestDB(t)
	user := createTestUserForSipUser(t, db)

	// 创建不同状态的SIP用户
	sipUsers := []*SipUser{
		{
			Username:   "registered-1",
			SchemeName: "Registered 1",
			Status:     SipUserStatusRegistered,
			UserID:     &user.ID,
		},
		{
			Username:   "registered-2",
			SchemeName: "Registered 2",
			Status:     SipUserStatusRegistered,
			UserID:     &user.ID,
		},
		{
			Username:   "unregistered-1",
			SchemeName: "Unregistered 1",
			Status:     SipUserStatusUnregistered,
			UserID:     &user.ID,
		},
		{
			Username:   "expired-1",
			SchemeName: "Expired 1",
			Status:     SipUserStatusExpired,
			UserID:     &user.ID,
		},
	}

	for _, sipUser := range sipUsers {
		err := CreateSipUser(db, sipUser)
		require.NoError(t, err)
	}

	// 获取已注册的用户
	registeredUsers, err := GetRegisteredSipUsers(db)
	assert.NoError(t, err)
	assert.Len(t, registeredUsers, 2)

	// 验证返回的都是已注册状态
	for _, user := range registeredUsers {
		assert.Equal(t, SipUserStatusRegistered, user.Status)
	}
}

func TestGetSipUsersByUserID(t *testing.T) {
	db := setupSipUserTestDB(t)
	user := createTestUserForSipUser(t, db)
	otherUser := &User{
		Email:    "other@example.com",
		Password: "hashedpassword",
	}
	err := db.Create(otherUser).Error
	require.NoError(t, err)

	// 为用户创建多个SIP用户（代接方案）
	sipUsers := []*SipUser{
		{
			Username:   "scheme-1",
			SchemeName: "工作模式",
			UserID:     &user.ID,
		},
		{
			Username:   "scheme-2",
			SchemeName: "会议模式",
			UserID:     &user.ID,
		},
		{
			Username:   "other-scheme",
			SchemeName: "其他用户方案",
			UserID:     &otherUser.ID,
		},
	}

	for _, sipUser := range sipUsers {
		err := CreateSipUser(db, sipUser)
		require.NoError(t, err)
	}

	// 获取指定用户的SIP用户列表
	userSipUsers, err := GetSipUsersByUserID(db, user.ID)
	assert.NoError(t, err)
	assert.Len(t, userSipUsers, 2)

	// 验证返回的都是指定用户的方案
	for _, sipUser := range userSipUsers {
		assert.Equal(t, user.ID, *sipUser.UserID)
	}

	// 验证方案名称
	schemeNames := make([]string, len(userSipUsers))
	for i, sipUser := range userSipUsers {
		schemeNames[i] = sipUser.SchemeName
	}
	assert.Contains(t, schemeNames, "工作模式")
	assert.Contains(t, schemeNames, "会议模式")
}

func TestActivateSipUser(t *testing.T) {
	db := setupSipUserTestDB(t)
	user := createTestUserForSipUser(t, db)

	// 创建多个SIP用户方案
	sipUsers := []*SipUser{
		{
			Username:   "scheme-1",
			SchemeName: "方案1",
			UserID:     &user.ID,
			IsActive:   true, // 初始激活状态
		},
		{
			Username:   "scheme-2",
			SchemeName: "方案2",
			UserID:     &user.ID,
			IsActive:   false,
		},
		{
			Username:   "scheme-3",
			SchemeName: "方案3",
			UserID:     &user.ID,
			IsActive:   false,
		},
	}

	for _, sipUser := range sipUsers {
		err := CreateSipUser(db, sipUser)
		require.NoError(t, err)
	}

	// 激活第二个方案
	err := ActivateSipUser(db, user.ID, sipUsers[1].ID)
	assert.NoError(t, err)

	// 验证激活状态
	activeSipUser, err := GetActiveSipUserByUserID(db, user.ID)
	assert.NoError(t, err)
	assert.NotNil(t, activeSipUser)
	assert.Equal(t, sipUsers[1].ID, activeSipUser.ID)
	assert.Equal(t, "方案2", activeSipUser.SchemeName)
	assert.True(t, activeSipUser.IsActive)

	// 验证其他方案已取消激活
	allUserSipUsers, err := GetSipUsersByUserID(db, user.ID)
	assert.NoError(t, err)

	activeCount := 0
	for _, sipUser := range allUserSipUsers {
		if sipUser.IsActive {
			activeCount++
			assert.Equal(t, sipUsers[1].ID, sipUser.ID)
		}
	}
	assert.Equal(t, 1, activeCount) // 只有一个激活的方案
}

func TestGetSipUserByPhoneNumber(t *testing.T) {
	db := setupSipUserTestDB(t)
	user := createTestUserForSipUser(t, db)

	// 创建绑定手机号的SIP用户
	sipUser := &SipUser{
		Username:         "phone-scheme",
		SchemeName:       "手机代接方案",
		UserID:           &user.ID,
		BoundPhoneNumber: "13800138000",
		Enabled:          true,
		IsActive:         true,
	}
	err := CreateSipUser(db, sipUser)
	require.NoError(t, err)

	// 创建未启用的方案
	disabledSipUser := &SipUser{
		Username:         "disabled-scheme",
		SchemeName:       "未启用方案",
		UserID:           &user.ID,
		BoundPhoneNumber: "13800138001",
		Enabled:          true, // Create as enabled first
		IsActive:         true,
	}
	err = CreateSipUser(db, disabledSipUser)
	require.NoError(t, err)

	// Then update to disabled to avoid GORM default value issue
	err = db.Model(disabledSipUser).Update("enabled", false).Error
	require.NoError(t, err)

	// 创建未激活的方案
	inactiveSipUser := &SipUser{
		Username:         "inactive-scheme",
		SchemeName:       "未激活方案",
		UserID:           &user.ID,
		BoundPhoneNumber: "13800138002",
		Enabled:          true,
		IsActive:         true, // Create as active first
	}
	err = CreateSipUser(db, inactiveSipUser)
	require.NoError(t, err)

	// Then update to inactive to avoid GORM default value issue
	err = db.Model(inactiveSipUser).Update("is_active", false).Error
	require.NoError(t, err)

	// 测试获取启用且激活的方案
	found, err := GetSipUserByPhoneNumber(db, "13800138000")
	assert.NoError(t, err)
	assert.NotNil(t, found)
	assert.Equal(t, sipUser.ID, found.ID)
	assert.Equal(t, "手机代接方案", found.SchemeName)

	// 测试获取未启用的方案（应该找不到）
	notFound, err := GetSipUserByPhoneNumber(db, "13800138001")
	assert.Error(t, err)
	assert.Nil(t, notFound)

	// 测试获取未激活的方案（应该找不到）
	notFound, err = GetSipUserByPhoneNumber(db, "13800138002")
	assert.Error(t, err)
	assert.Nil(t, notFound)

	// 测试获取不存在的手机号
	notFound, err = GetSipUserByPhoneNumber(db, "13800138999")
	assert.Error(t, err)
	assert.Nil(t, notFound)
}

func TestSipUser_RecordingModes(t *testing.T) {
	db := setupSipUserTestDB(t)
	user := createTestUserForSipUser(t, db)

	modes := []RecordingMode{
		RecordingModeDisabled,
		RecordingModeFull,
		RecordingModeMessage,
	}

	for i, mode := range modes {
		sipUser := &SipUser{
			Username:      "recording-" + string(rune('1'+i)),
			SchemeName:    "Recording Mode " + string(mode),
			UserID:        &user.ID,
			RecordingMode: mode,
		}
		err := CreateSipUser(db, sipUser)
		require.NoError(t, err)

		retrieved, err := GetSipUserByID(db, sipUser.ID)
		assert.NoError(t, err)
		assert.Equal(t, mode, retrieved.RecordingMode)
	}
}

func TestSipUser_WithGroup(t *testing.T) {
	db := setupSipUserTestDB(t)
	user := createTestUserForSipUser(t, db)

	// 创建组织
	group := &Group{
		Name:      "Test Company",
		Type:      "company",
		CreatorID: user.ID,
	}
	err := db.Create(group).Error
	require.NoError(t, err)

	// 创建组织的SIP用户
	sipUser := &SipUser{
		Username:   "company-scheme",
		SchemeName: "公司代接方案",
		UserID:     &user.ID,
		GroupID:    &group.ID,
	}
	err = CreateSipUser(db, sipUser)
	require.NoError(t, err)

	// 验证组织关联
	retrieved, err := GetSipUserByID(db, sipUser.ID)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved.GroupID)
	assert.Equal(t, group.ID, *retrieved.GroupID)

	// 测试按组织ID获取SIP用户
	groupSipUsers, err := GetSipUsersByGroupID(db, group.ID)
	assert.NoError(t, err)
	assert.Len(t, groupSipUsers, 1)
	assert.Equal(t, sipUser.ID, groupSipUsers[0].ID)
}

func TestSipUser_ComplexKeywordReplies(t *testing.T) {
	db := setupSipUserTestDB(t)
	user := createTestUserForSipUser(t, db)

	complexKeywords := KeywordReplies{
		{Keyword: "价格", Reply: "关于价格信息，我们有多种套餐可选，请问您需要了解哪种？"},
		{Keyword: "地址", Reply: "我们的地址是北京市朝阳区xxx路xxx号，地铁x号线xxx站A出口。"},
		{Keyword: "营业时间", Reply: "我们的营业时间是周一至周五9:00-18:00，周末10:00-17:00。"},
		{Keyword: "联系方式", Reply: "您可以拨打400-xxx-xxxx或发送邮件至service@example.com联系我们。"},
		{Keyword: "退款", Reply: "关于退款政策，请稍等，我为您转接专门的客服人员。"},
	}

	sipUser := &SipUser{
		Username:       "complex-keywords",
		SchemeName:     "复杂关键词方案",
		UserID:         &user.ID,
		KeywordReplies: complexKeywords,
	}

	err := CreateSipUser(db, sipUser)
	assert.NoError(t, err)

	retrieved, err := GetSipUserByID(db, sipUser.ID)
	assert.NoError(t, err)
	assert.Len(t, retrieved.KeywordReplies, 5)

	// 验证每个关键词回复
	keywordMap := make(map[string]string)
	for _, kr := range retrieved.KeywordReplies {
		keywordMap[kr.Keyword] = kr.Reply
	}

	assert.Contains(t, keywordMap["价格"], "多种套餐")
	assert.Contains(t, keywordMap["地址"], "朝阳区")
	assert.Contains(t, keywordMap["营业时间"], "9:00-18:00")
	assert.Contains(t, keywordMap["联系方式"], "400-xxx-xxxx")
	assert.Contains(t, keywordMap["退款"], "转接专门的客服")
}

func TestSipUser_Statistics(t *testing.T) {
	db := setupSipUserTestDB(t)
	user := createTestUserForSipUser(t, db)

	sipUser := &SipUser{
		Username:          "stats-test",
		SchemeName:        "统计测试方案",
		UserID:            &user.ID,
		RegisterCount:     100,
		CallCount:         50,
		TotalCallDuration: 7200, // 2小时
		MessageCount:      15,
	}

	err := CreateSipUser(db, sipUser)
	require.NoError(t, err)

	// 模拟统计数据更新
	sipUser.RegisterCount += 10
	sipUser.CallCount += 5
	sipUser.TotalCallDuration += 600 // 增加10分钟
	sipUser.MessageCount += 2

	err = UpdateSipUser(db, sipUser)
	assert.NoError(t, err)

	retrieved, err := GetSipUserByID(db, sipUser.ID)
	assert.NoError(t, err)
	assert.Equal(t, 110, retrieved.RegisterCount)
	assert.Equal(t, 55, retrieved.CallCount)
	assert.Equal(t, 7800, retrieved.TotalCallDuration)
	assert.Equal(t, 17, retrieved.MessageCount)
}

// Benchmark tests
func BenchmarkCreateSipUser(b *testing.B) {
	db := setupSipUserTestDB(&testing.T{})
	user := createTestUserForSipUser(&testing.T{}, db)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sipUser := &SipUser{
			Username:   "benchmark-" + string(rune(i)),
			SchemeName: "Benchmark Scheme " + string(rune(i)),
			UserID:     &user.ID,
		}
		CreateSipUser(db, sipUser)
	}
}

func BenchmarkGetSipUserByUsername(b *testing.B) {
	db := setupSipUserTestDB(&testing.T{})
	user := createTestUserForSipUser(&testing.T{}, db)

	sipUser := &SipUser{
		SchemeName: "Benchmark Scheme",
		UserID:     &user.ID,
	}
	CreateSipUser(db, sipUser)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetSipUserByUsername(db, "benchmark-user")
	}
}

func BenchmarkKeywordReplies_Value(b *testing.B) {
	kr := KeywordReplies{
		{Keyword: "hello", Reply: "Hi there!"},
		{Keyword: "help", Reply: "How can I assist you?"},
		{Keyword: "price", Reply: "Let me check the pricing for you."},
		{Keyword: "address", Reply: "Our address is 123 Main St."},
		{Keyword: "hours", Reply: "We're open 9-5 Monday through Friday."},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		kr.Value()
	}
}
