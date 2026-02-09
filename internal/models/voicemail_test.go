package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupVoicemailTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&User{}, &SipUser{}, &SipCall{}, &Voicemail{})
	require.NoError(t, err)

	return db
}

func createTestUserForVoicemail(t *testing.T, db *gorm.DB) *User {
	user := &User{
		Email:    "voicemail@example.com",
		Password: "hashedpassword",
	}
	err := db.Create(user).Error
	require.NoError(t, err)
	return user
}

func createTestSipUserForVoicemail(t *testing.T, db *gorm.DB, userID uint) *SipUser {
	sipUser := &SipUser{
		UserID:     &userID,
		SchemeName: "Voicemail Scheme",
		Status:     SipUserStatusRegistered,
	}
	err := db.Create(sipUser).Error
	require.NoError(t, err)
	return sipUser
}

func createTestSipCallForVoicemail(t *testing.T, db *gorm.DB, userID uint) *SipCall {
	sipCall := &SipCall{
		CallID:    "voicemail-call@example.com",
		Direction: SipCallDirectionInbound,
		Status:    SipCallStatusEnded,
		StartTime: time.Now(),
		UserID:    &userID,
	}
	err := db.Create(sipCall).Error
	require.NoError(t, err)
	return sipCall
}

func TestVoicemail_CRUD(t *testing.T) {
	db := setupVoicemailTestDB(t)
	user := createTestUserForVoicemail(t, db)
	sipUser := createTestSipUserForVoicemail(t, db, user.ID)
	sipCall := createTestSipCallForVoicemail(t, db, user.ID)

	// 测试创建留言
	voicemail := &Voicemail{
		UserID:           user.ID,
		SipUserID:        &sipUser.ID,
		SipCallID:        &sipCall.ID,
		CallerNumber:     "13800138000",
		CallerName:       "张三",
		CallerLocation:   "北京市朝阳区",
		AudioPath:        "/voicemails/2024/01/15/vm_001.wav",
		AudioURL:         "https://example.com/voicemails/vm_001.wav",
		AudioFormat:      "wav",
		AudioSize:        1024000,
		Duration:         30,
		SampleRate:       8000,
		Channels:         1,
		TranscribedText:  "您好，我想咨询一下贵公司的产品信息，请回电话给我，谢谢。",
		Summary:          "客户咨询产品信息，请求回电",
		Keywords:         `["产品", "咨询", "回电"]`,
		Status:           VoicemailStatusNew,
		IsRead:           false,
		IsImportant:      false,
		TranscribeStatus: "completed",
		Metadata:         `{"quality": "good", "noise_level": "low"}`,
		Notes:            "重要客户留言",
	}

	err := CreateVoicemail(db, voicemail)
	assert.NoError(t, err)
	assert.NotZero(t, voicemail.ID)
	assert.NotZero(t, voicemail.CreatedAt)

	// 测试读取留言
	retrieved, err := GetVoicemailByID(db, voicemail.ID)
	assert.NoError(t, err)
	assert.NotNil(t, retrieved)
	assert.Equal(t, user.ID, retrieved.UserID)
	assert.Equal(t, sipUser.ID, *retrieved.SipUserID)
	assert.Equal(t, sipCall.ID, *retrieved.SipCallID)
	assert.Equal(t, "13800138000", retrieved.CallerNumber)
	assert.Equal(t, "张三", retrieved.CallerName)
	assert.Equal(t, "北京市朝阳区", retrieved.CallerLocation)
	assert.Equal(t, "/voicemails/2024/01/15/vm_001.wav", retrieved.AudioPath)
	assert.Equal(t, "https://example.com/voicemails/vm_001.wav", retrieved.AudioURL)
	assert.Equal(t, "wav", retrieved.AudioFormat)
	assert.Equal(t, int64(1024000), retrieved.AudioSize)
	assert.Equal(t, 30, retrieved.Duration)
	assert.Equal(t, 8000, retrieved.SampleRate)
	assert.Equal(t, 1, retrieved.Channels)
	assert.Equal(t, "您好，我想咨询一下贵公司的产品信息，请回电话给我，谢谢。", retrieved.TranscribedText)
	assert.Equal(t, "客户咨询产品信息，请求回电", retrieved.Summary)
	assert.Equal(t, `["产品", "咨询", "回电"]`, retrieved.Keywords)
	assert.Equal(t, VoicemailStatusNew, retrieved.Status)
	assert.False(t, retrieved.IsRead)
	assert.False(t, retrieved.IsImportant)
	assert.Equal(t, "completed", retrieved.TranscribeStatus)
	assert.Equal(t, user.Email, retrieved.User.Email)
	assert.Equal(t, sipUser.Username, retrieved.SipUser.Username)
	assert.Equal(t, sipCall.CallID, retrieved.SipCall.CallID)

	// 测试更新留言
	retrieved.Summary = "更新后的摘要"
	retrieved.IsImportant = true
	retrieved.Notes = "更新后的备注"
	err = UpdateVoicemail(db, retrieved)
	assert.NoError(t, err)

	updated, err := GetVoicemailByID(db, voicemail.ID)
	assert.NoError(t, err)
	assert.Equal(t, "更新后的摘要", updated.Summary)
	assert.True(t, updated.IsImportant)
	assert.Equal(t, "更新后的备注", updated.Notes)

	// 测试软删除
	err = DeleteVoicemail(db, voicemail.ID)
	assert.NoError(t, err)

	deleted, err := GetVoicemailByID(db, voicemail.ID)
	assert.Error(t, err)
	assert.Nil(t, deleted)
}

func TestGetVoicemailsByUserID(t *testing.T) {
	db := setupVoicemailTestDB(t)
	user := createTestUserForVoicemail(t, db)
	sipUser := createTestSipUserForVoicemail(t, db, user.ID)

	// 创建多个留言
	voicemails := []*Voicemail{
		{
			UserID:       user.ID,
			SipUserID:    &sipUser.ID,
			CallerNumber: "13800138001",
			AudioPath:    "/voicemails/vm_001.wav",
			Duration:     20,
			Status:       VoicemailStatusNew,
		},
		{
			UserID:       user.ID,
			SipUserID:    &sipUser.ID,
			CallerNumber: "13800138002",
			AudioPath:    "/voicemails/vm_002.wav",
			Duration:     35,
			Status:       VoicemailStatusRead,
		},
		{
			UserID:       user.ID,
			SipUserID:    &sipUser.ID,
			CallerNumber: "13800138003",
			AudioPath:    "/voicemails/vm_003.wav",
			Duration:     15,
			Status:       VoicemailStatusNew,
		},
	}

	for _, vm := range voicemails {
		err := CreateVoicemail(db, vm)
		require.NoError(t, err)
		time.Sleep(time.Millisecond) // 确保创建时间不同
	}

	// 获取用户的留言列表
	userVoicemails, total, err := GetVoicemailsByUserID(db, user.ID, 10, 0)
	assert.NoError(t, err)
	assert.Len(t, userVoicemails, 3)
	assert.Equal(t, int64(3), total)

	// 验证按创建时间倒序排列
	assert.Equal(t, "13800138003", userVoicemails[0].CallerNumber) // 最新的
	assert.Equal(t, "13800138002", userVoicemails[1].CallerNumber)
	assert.Equal(t, "13800138001", userVoicemails[2].CallerNumber) // 最早的

	// 测试分页
	pagedVoicemails, pagedTotal, err := GetVoicemailsByUserID(db, user.ID, 2, 1)
	assert.NoError(t, err)
	assert.Len(t, pagedVoicemails, 2)
	assert.Equal(t, int64(3), pagedTotal)
	assert.Equal(t, "13800138002", pagedVoicemails[0].CallerNumber)
	assert.Equal(t, "13800138001", pagedVoicemails[1].CallerNumber)
}

func TestGetUnreadVoicemailsCount(t *testing.T) {
	db := setupVoicemailTestDB(t)
	user := createTestUserForVoicemail(t, db)
	sipUser := createTestSipUserForVoicemail(t, db, user.ID)

	// 创建已读和未读留言
	voicemails := []*Voicemail{
		{
			UserID:       user.ID,
			SipUserID:    &sipUser.ID,
			CallerNumber: "13800138001",
			AudioPath:    "/voicemails/vm_001.wav",
			IsRead:       false,
			Status:       VoicemailStatusNew,
		},
		{
			UserID:       user.ID,
			SipUserID:    &sipUser.ID,
			CallerNumber: "13800138002",
			AudioPath:    "/voicemails/vm_002.wav",
			IsRead:       true,
			Status:       VoicemailStatusRead,
		},
		{
			UserID:       user.ID,
			SipUserID:    &sipUser.ID,
			CallerNumber: "13800138003",
			AudioPath:    "/voicemails/vm_003.wav",
			IsRead:       false,
			Status:       VoicemailStatusNew,
		},
	}

	for _, vm := range voicemails {
		err := CreateVoicemail(db, vm)
		require.NoError(t, err)
	}

	// 获取未读留言数量
	unreadCount, err := GetUnreadVoicemailsCount(db, user.ID)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), unreadCount)
}

func TestMarkVoicemailAsRead(t *testing.T) {
	db := setupVoicemailTestDB(t)
	user := createTestUserForVoicemail(t, db)
	sipUser := createTestSipUserForVoicemail(t, db, user.ID)

	// 创建未读留言
	voicemail := &Voicemail{
		UserID:       user.ID,
		SipUserID:    &sipUser.ID,
		CallerNumber: "13800138001",
		AudioPath:    "/voicemails/vm_001.wav",
		IsRead:       false,
		Status:       VoicemailStatusNew,
	}

	err := CreateVoicemail(db, voicemail)
	require.NoError(t, err)

	// 标记为已读
	err = MarkVoicemailAsRead(db, voicemail.ID)
	assert.NoError(t, err)

	// 验证已标记为已读
	updated, err := GetVoicemailByID(db, voicemail.ID)
	assert.NoError(t, err)
	assert.True(t, updated.IsRead)
	assert.Equal(t, VoicemailStatusRead, updated.Status)
	assert.NotNil(t, updated.ReadAt)
}

func TestGetVoicemailsByStatus(t *testing.T) {
	db := setupVoicemailTestDB(t)
	user := createTestUserForVoicemail(t, db)
	sipUser := createTestSipUserForVoicemail(t, db, user.ID)

	// 创建不同状态的留言
	statuses := []VoicemailStatus{
		VoicemailStatusNew,
		VoicemailStatusRead,
		VoicemailStatusArchived,
		VoicemailStatusNew,
		VoicemailStatusRead,
	}

	for i, status := range statuses {
		voicemail := &Voicemail{
			UserID:       user.ID,
			SipUserID:    &sipUser.ID,
			CallerNumber: "1380013800" + string(rune('1'+i)),
			AudioPath:    "/voicemails/vm_00" + string(rune('1'+i)) + ".wav",
			Status:       status,
		}
		err := CreateVoicemail(db, voicemail)
		require.NoError(t, err)
	}

	// 按状态查询
	newVoicemails, newTotal, err := GetVoicemailsByStatus(db, user.ID, VoicemailStatusNew, 10, 0)
	assert.NoError(t, err)
	assert.Len(t, newVoicemails, 2)
	assert.Equal(t, int64(2), newTotal)

	readVoicemails, readTotal, err := GetVoicemailsByStatus(db, user.ID, VoicemailStatusRead, 10, 0)
	assert.NoError(t, err)
	assert.Len(t, readVoicemails, 2)
	assert.Equal(t, int64(2), readTotal)

	archivedVoicemails, archivedTotal, err := GetVoicemailsByStatus(db, user.ID, VoicemailStatusArchived, 10, 0)
	assert.NoError(t, err)
	assert.Len(t, archivedVoicemails, 1)
	assert.Equal(t, int64(1), archivedTotal)
}

func TestGetVoicemailsByCallerNumber(t *testing.T) {
	db := setupVoicemailTestDB(t)
	user := createTestUserForVoicemail(t, db)
	sipUser := createTestSipUserForVoicemail(t, db, user.ID)

	callerNumber := "13800138000"

	// 创建同一来电号码的多个留言
	for i := 0; i < 3; i++ {
		voicemail := &Voicemail{
			UserID:       user.ID,
			SipUserID:    &sipUser.ID,
			CallerNumber: callerNumber,
			AudioPath:    "/voicemails/vm_00" + string(rune('1'+i)) + ".wav",
			Duration:     20 + i*5,
			Status:       VoicemailStatusNew,
		}
		err := CreateVoicemail(db, voicemail)
		require.NoError(t, err)
		time.Sleep(time.Millisecond)
	}

	// 创建其他号码的留言
	otherVoicemail := &Voicemail{
		UserID:       user.ID,
		SipUserID:    &sipUser.ID,
		CallerNumber: "13800138999",
		AudioPath:    "/voicemails/vm_other.wav",
		Status:       VoicemailStatusNew,
	}
	err := CreateVoicemail(db, otherVoicemail)
	require.NoError(t, err)

	// 按来电号码查询
	callerVoicemails, total, err := GetVoicemailsByCallerNumber(db, user.ID, callerNumber, 10, 0)
	assert.NoError(t, err)
	assert.Len(t, callerVoicemails, 3)
	assert.Equal(t, int64(3), total)

	// 验证都是指定号码的留言
	for _, vm := range callerVoicemails {
		assert.Equal(t, callerNumber, vm.CallerNumber)
	}

	// 验证按时间倒序排列
	assert.True(t, callerVoicemails[0].CreatedAt.After(callerVoicemails[1].CreatedAt))
	assert.True(t, callerVoicemails[1].CreatedAt.After(callerVoicemails[2].CreatedAt))
}

func TestVoicemail_TranscriptionStatus(t *testing.T) {
	db := setupVoicemailTestDB(t)
	user := createTestUserForVoicemail(t, db)
	sipUser := createTestSipUserForVoicemail(t, db, user.ID)

	// 测试转录状态转换
	voicemail := &Voicemail{
		UserID:           user.ID,
		SipUserID:        &sipUser.ID,
		CallerNumber:     "13800138000",
		AudioPath:        "/voicemails/transcription_test.wav",
		TranscribeStatus: "pending",
	}

	err := CreateVoicemail(db, voicemail)
	require.NoError(t, err)

	// 状态转换：pending -> processing
	voicemail.TranscribeStatus = "processing"
	err = UpdateVoicemail(db, voicemail)
	assert.NoError(t, err)

	// 状态转换：processing -> completed
	transcribedAt := time.Now()
	voicemail.TranscribeStatus = "completed"
	voicemail.TranscribedAt = &transcribedAt
	voicemail.TranscribedText = "这是转录的文本内容"
	voicemail.Summary = "转录摘要"
	err = UpdateVoicemail(db, voicemail)
	assert.NoError(t, err)

	// 验证转录完成状态
	completed, err := GetVoicemailByID(db, voicemail.ID)
	assert.NoError(t, err)
	assert.Equal(t, "completed", completed.TranscribeStatus)
	assert.NotNil(t, completed.TranscribedAt)
	assert.Equal(t, "这是转录的文本内容", completed.TranscribedText)
	assert.Equal(t, "转录摘要", completed.Summary)

	// 测试转录失败
	failedVoicemail := &Voicemail{
		UserID:           user.ID,
		SipUserID:        &sipUser.ID,
		CallerNumber:     "13800138001",
		AudioPath:        "/voicemails/failed_transcription.wav",
		TranscribeStatus: "failed",
		TranscribeError:  "音频质量太低，无法转录",
	}

	err = CreateVoicemail(db, failedVoicemail)
	assert.NoError(t, err)

	failed, err := GetVoicemailByID(db, failedVoicemail.ID)
	assert.NoError(t, err)
	assert.Equal(t, "failed", failed.TranscribeStatus)
	assert.Equal(t, "音频质量太低，无法转录", failed.TranscribeError)
}

func TestVoicemail_AudioFormats(t *testing.T) {
	db := setupVoicemailTestDB(t)
	user := createTestUserForVoicemail(t, db)
	sipUser := createTestSipUserForVoicemail(t, db, user.ID)

	formats := []struct {
		format     string
		sampleRate int
		channels   int
	}{
		{"wav", 8000, 1},
		{"mp3", 16000, 1},
		{"opus", 48000, 2},
		{"aac", 44100, 2},
	}

	for i, f := range formats {
		voicemail := &Voicemail{
			UserID:       user.ID,
			SipUserID:    &sipUser.ID,
			CallerNumber: "1380013800" + string(rune('1'+i)),
			AudioPath:    "/voicemails/vm_" + f.format + ".wav",
			AudioFormat:  f.format,
			SampleRate:   f.sampleRate,
			Channels:     f.channels,
		}

		err := CreateVoicemail(db, voicemail)
		require.NoError(t, err)

		retrieved, err := GetVoicemailByID(db, voicemail.ID)
		assert.NoError(t, err)
		assert.Equal(t, f.format, retrieved.AudioFormat)
		assert.Equal(t, f.sampleRate, retrieved.SampleRate)
		assert.Equal(t, f.channels, retrieved.Channels)
	}
}

func TestVoicemail_ImportanceAndArchiving(t *testing.T) {
	db := setupVoicemailTestDB(t)
	user := createTestUserForVoicemail(t, db)
	sipUser := createTestSipUserForVoicemail(t, db, user.ID)

	voicemail := &Voicemail{
		UserID:       user.ID,
		SipUserID:    &sipUser.ID,
		CallerNumber: "13800138000",
		AudioPath:    "/voicemails/important.wav",
		Status:       VoicemailStatusNew,
		IsImportant:  false,
	}

	err := CreateVoicemail(db, voicemail)
	require.NoError(t, err)

	// 标记为重要
	voicemail.IsImportant = true
	err = UpdateVoicemail(db, voicemail)
	assert.NoError(t, err)

	// 归档留言
	voicemail.Status = VoicemailStatusArchived
	err = UpdateVoicemail(db, voicemail)
	assert.NoError(t, err)

	// 验证状态
	updated, err := GetVoicemailByID(db, voicemail.ID)
	assert.NoError(t, err)
	assert.True(t, updated.IsImportant)
	assert.Equal(t, VoicemailStatusArchived, updated.Status)
}

func TestVoicemail_Metadata(t *testing.T) {
	db := setupVoicemailTestDB(t)
	user := createTestUserForVoicemail(t, db)
	sipUser := createTestSipUserForVoicemail(t, db, user.ID)

	metadata := `{
		"audio_quality": "good",
		"noise_level": "low",
		"speaker_count": 1,
		"language": "zh-CN",
		"confidence": 0.95
	}`

	voicemail := &Voicemail{
		UserID:       user.ID,
		SipUserID:    &sipUser.ID,
		CallerNumber: "13800138000",
		AudioPath:    "/voicemails/metadata_test.wav",
		Metadata:     metadata,
		Notes:        "包含详细元数据的留言",
	}

	err := CreateVoicemail(db, voicemail)
	require.NoError(t, err)

	retrieved, err := GetVoicemailByID(db, voicemail.ID)
	assert.NoError(t, err)
	assert.JSONEq(t, metadata, retrieved.Metadata)
	assert.Equal(t, "包含详细元数据的留言", retrieved.Notes)
}

func TestVoicemail_ErrorHandling(t *testing.T) {
	db := setupVoicemailTestDB(t)

	// 测试获取不存在的留言
	notFound, err := GetVoicemailByID(db, 99999)
	assert.Error(t, err)
	assert.Nil(t, notFound)

	// 测试获取不存在用户的留言
	emptyVoicemails, total, err := GetVoicemailsByUserID(db, 99999, 10, 0)
	assert.NoError(t, err)
	assert.Len(t, emptyVoicemails, 0)
	assert.Equal(t, int64(0), total)

	// 测试获取不存在用户的未读留言数量
	count, err := GetUnreadVoicemailsCount(db, 99999)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

// Benchmark tests
func BenchmarkCreateVoicemail(b *testing.B) {
	db := setupVoicemailTestDB(&testing.T{})
	user := createTestUserForVoicemail(&testing.T{}, db)
	sipUser := createTestSipUserForVoicemail(&testing.T{}, db, user.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		voicemail := &Voicemail{
			UserID:       user.ID,
			SipUserID:    &sipUser.ID,
			CallerNumber: "1380013800" + string(rune('0'+i%10)),
			AudioPath:    "/voicemails/benchmark_" + string(rune(i)) + ".wav",
			Duration:     30,
			Status:       VoicemailStatusNew,
		}
		CreateVoicemail(db, voicemail)
	}
}

func BenchmarkGetVoicemailsByUserID(b *testing.B) {
	db := setupVoicemailTestDB(&testing.T{})
	user := createTestUserForVoicemail(&testing.T{}, db)
	sipUser := createTestSipUserForVoicemail(&testing.T{}, db, user.ID)

	// 创建测试数据
	for i := 0; i < 100; i++ {
		voicemail := &Voicemail{
			UserID:       user.ID,
			SipUserID:    &sipUser.ID,
			CallerNumber: "1380013800" + string(rune('0'+i%10)),
			AudioPath:    "/voicemails/test_" + string(rune(i)) + ".wav",
			Duration:     30,
			Status:       VoicemailStatusNew,
		}
		CreateVoicemail(db, voicemail)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetVoicemailsByUserID(db, user.ID, 10, 0)
	}
}
