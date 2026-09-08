package db

import "context"

type CreateUserTxParams struct {
	CreateUserParams
	AfterCreate func(user User) error
}

type CreateUserTxResult struct {
	User User
	// AfterCreateErr holds any error returned by AfterCreate. Unlike an
	// error returned from the transaction function itself, an
	// AfterCreateErr does NOT cause the user-creation transaction to roll
	// back -- the user row is still committed. Callers should check this
	// field to detect and surface (e.g. log) a failure in the
	// post-creation step separately from the user-creation result.
	AfterCreateErr error
}

func (store *SQLStore) CreateUserTx(ctx context.Context, arg CreateUserTxParams) (CreateUserTxResult, error) {
	var result CreateUserTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		var err error

		result.User, err = q.CreateUser(ctx, arg.CreateUserParams)
		if err != nil {
			return err
		}

		// AfterCreate's error is captured separately instead of being
		// returned here: returning it would make execTx roll back the
		// transaction, discarding the newly created user even though the
		// SQL insert itself succeeded. A failure in AfterCreate (e.g. the
		// verify-email task queue being unavailable) should not prevent
		// the user from being created.
		result.AfterCreateErr = arg.AfterCreate(result.User)
		return nil
	})

	return result, err
}
