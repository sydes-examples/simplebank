package worker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
	mockdb "github.com/techschool/simplebank/db/mock"
	db "github.com/techschool/simplebank/db/sqlc"
	"github.com/techschool/simplebank/util"
)

// mockEmailSender is a hand-written mock of mail.EmailSender. There is no
// existing mockmail package in the repo, and the interface has a single
// method, so a small hand-rolled mock is simpler than wiring up gomock/
// mockgen for it.
type mockEmailSender struct {
	calls int
}

func (m *mockEmailSender) SendEmail(
	subject string,
	content string,
	to []string,
	cc []string,
	bcc []string,
	attachFiles []string,
) error {
	m.calls++
	return nil
}

// newTestRedisTaskProcessor builds a *RedisTaskProcessor directly from its
// fields so ProcessTaskSendVerifyEmail can be exercised without a live
// Redis connection. The server field is left nil since it is only used by
// Start/Shutdown, neither of which the task handler under test touches.
// This works because the test lives in package worker and can see the
// struct's unexported fields.
func newTestRedisTaskProcessor(store db.Store, mailer *mockEmailSender) *RedisTaskProcessor {
	return &RedisTaskProcessor{
		store:  store,
		mailer: mailer,
	}
}

func randomVerifyEmailUser(t *testing.T) db.User {
	hashedPassword, err := util.HashPassword(util.RandomString(6))
	require.NoError(t, err)

	return db.User{
		Username:          util.RandomOwner(),
		HashedPassword:    hashedPassword,
		FullName:          util.RandomOwner(),
		Email:             util.RandomEmail(),
		PasswordChangedAt: time.Now(),
		CreatedAt:         time.Now(),
		IsEmailVerified:   false,
		Role:              util.DepositorRole,
	}
}

func newSendVerifyEmailTask(t *testing.T, username string) *asynq.Task {
	payload, err := json.Marshal(PayloadSendVerifyEmail{Username: username})
	require.NoError(t, err)
	return asynq.NewTask(TaskSendVerifyEmail, payload)
}

func TestProcessTaskSendVerifyEmail(t *testing.T) {
	t.Run("AlreadyVerified", func(t *testing.T) {
		user := randomVerifyEmailUser(t)
		user.IsEmailVerified = true

		storeCtrl := gomock.NewController(t)
		defer storeCtrl.Finish()
		store := mockdb.NewMockStore(storeCtrl)

		store.EXPECT().
			GetUser(gomock.Any(), gomock.Eq(user.Username)).
			Times(1).
			Return(user, nil)

		// The precondition should short-circuit before either of these is
		// reached, so they must never be called.
		store.EXPECT().
			CreateVerifyEmail(gomock.Any(), gomock.Any()).
			Times(0)

		mailer := &mockEmailSender{}
		processor := newTestRedisTaskProcessor(store, mailer)

		task := newSendVerifyEmailTask(t, user.Username)
		err := processor.ProcessTaskSendVerifyEmail(context.Background(), task)

		require.NoError(t, err)
		require.Equal(t, 0, mailer.calls)
	})

	t.Run("NotYetVerified", func(t *testing.T) {
		user := randomVerifyEmailUser(t)
		user.IsEmailVerified = false

		storeCtrl := gomock.NewController(t)
		defer storeCtrl.Finish()
		store := mockdb.NewMockStore(storeCtrl)

		store.EXPECT().
			GetUser(gomock.Any(), gomock.Eq(user.Username)).
			Times(1).
			Return(user, nil)

		verifyEmail := db.VerifyEmail{
			ID:         1,
			Username:   user.Username,
			Email:      user.Email,
			SecretCode: util.RandomString(32),
		}
		store.EXPECT().
			CreateVerifyEmail(gomock.Any(), gomock.Any()).
			Times(1).
			Return(verifyEmail, nil)

		mailer := &mockEmailSender{}
		processor := newTestRedisTaskProcessor(store, mailer)

		task := newSendVerifyEmailTask(t, user.Username)
		err := processor.ProcessTaskSendVerifyEmail(context.Background(), task)

		require.NoError(t, err)
		require.Equal(t, 1, mailer.calls)
	})
}
