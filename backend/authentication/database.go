package main

import (
	"context"
	"database/sql"
)

/**************
** USER CRUD **
***************/

// func(context, current open transaction, email, password hash, name, alias, googleId)
//   - inserts a user into the users table with the given email, password hash, name, alias, and google ID in the given transaction
//   - returns: the new user's ID or an error
//   - assumes: you will close this transaction in the parent function!
func createUser(ctx context.Context, tx *sql.Tx, email string, passwordHash string, name string, googleId string) (string, error) {
	var userId string
	googleIdNull := sql.NullString{String: googleId, Valid: googleId != ""}
	passwordHashNull := sql.NullString{String: passwordHash, Valid: passwordHash != ""}
	err := tx.QueryRowContext(
		ctx,
		"INSERT INTO users (email, password_hash, name, alias, google_id) VALUES ($1, $2, $3, $4, $5) RETURNING id",
		email,
		passwordHashNull,
		name,
		name,
		googleIdNull,
	).Scan(&userId)
	if err != nil {
		return "", err
	}
	return userId, nil
}

// func(context, current open transaction, user ID)
//   - fetches a user's email, password hash, name, alias, avatar number, and Google ID from the users table with the given user ID in the given transaction
//   - returns: email, password hash, name, alias, avatar number, Google ID, or an error
//   - assumes: you will close this transaction in the parent function!
func getUserById(ctx context.Context, tx *sql.Tx, userId string) (string, string, string, string, int, string, error) {
	var email, name, alias string
	var passwordHash sql.NullString
	var avatarNum int
	var googleId sql.NullString
	err := tx.QueryRowContext(
		ctx,
		"SELECT email, password_hash, name, alias, avatar_num, google_id FROM users WHERE id = $1",
		userId,
	).Scan(&email, &passwordHash, &name, &alias, &avatarNum, &googleId)
	if err != nil {
		return "", "", "", "", 0, "", err
	}
	actualPasswordHash := ""
	if passwordHash.Valid {
		actualPasswordHash = passwordHash.String
	}
	actualGoogleId := ""
	if googleId.Valid {
		actualGoogleId = googleId.String
	}
	return email, actualPasswordHash, name, alias, avatarNum, actualGoogleId, nil
}

// func(context, current open transaction, email)
//   - fetches a user's id, password hash, name, alias, avatar number, and Google ID from the users table with the given email in the given transaction
//   - returns: user ID, password hash, name, alias, avatar number, Google ID, or an error
//   - assumes: you will close this transaction in the parent function!
func getUserByEmail(ctx context.Context, tx *sql.Tx, email string) (string, string, string, string, int, string, error) {
	var userId, name, alias string
	var passwordHash, googleId sql.NullString
	var avatarNum int
	err := tx.QueryRowContext(
		ctx,
		"SELECT id, password_hash, name, alias, avatar_num, google_id FROM users WHERE email = $1",
		email,
	).Scan(&userId, &passwordHash, &name, &alias, &avatarNum, &googleId)
	if err != nil {
		return "", "", "", "", 0, "", err
	}

	actualPasswordHash := ""
	if passwordHash.Valid {
		actualPasswordHash = passwordHash.String
	}

	actualGoogleId := ""
	if googleId.Valid {
		actualGoogleId = googleId.String
	}

	return userId, actualPasswordHash, name, alias, avatarNum, actualGoogleId, nil
}

// func(context, current open transaction, googleId)
//   - fetches a user's id, email, password hash, name, alias, avatar number, and Google ID from the users table with the given Google ID in the given transaction
//   - returns: user ID, email, password hash, name, alias, avatar number, Google ID, or an error
//   - assumes: you will close this transaction in the parent function!
func getUserByGoogleId(ctx context.Context, tx *sql.Tx, googleId string) (string, string, string, string, string, int, string, error) {
	var userId, email, name, alias string
	var passwordHash, dbGoogleId sql.NullString
	var avatarNum int
	err := tx.QueryRowContext(
		ctx,
		"SELECT id, email, password_hash, name, alias, avatar_num, google_id FROM users WHERE google_id = $1",
		googleId,
	).Scan(&userId, &email, &passwordHash, &name, &alias, &avatarNum, &dbGoogleId)
	if err != nil {
		return "", "", "", "", "", 0, "", err
	}

	actualPasswordHash := ""
	if passwordHash.Valid {
		actualPasswordHash = passwordHash.String
	}
	actualGoogleId := ""
	if dbGoogleId.Valid {
		actualGoogleId = dbGoogleId.String
	}

	return userId, email, actualPasswordHash, name, alias, avatarNum, actualGoogleId, nil
}

// func(context, current open transaction, user ID, email, password hash, name, alias, avatar number, Google ID)
//   - updates a user's email, password hash, name, alias, avatar number, and Google ID in the users table with the given user ID in the given transaction
//   - returns: the updated user's ID or an error
//   - assumes: you will close this transaction in the parent function!
func updateUser(ctx context.Context, tx *sql.Tx, userId string, email string, passwordHash sql.NullString, name string, alias string, avatarNum int, googleId sql.NullString) (string, error) {
	_, err := tx.ExecContext(
		ctx,
		`UPDATE users SET email = $1, password_hash = $2, name = $3, alias = $4, avatar_num = $5, google_id = $6 WHERE id = $7`,
		email,
		passwordHash,
		name,
		alias,
		avatarNum,
		googleId,
		userId,
	)
	if err != nil {
		return "", err
	}
	return userId, nil
}

// func(context, current open transaction, user ID)
//   - deletes a user's account from the users table with the given user ID in the given transaction
//   - returns: an error if the deletion fails
//   - assumes: you will close this transaction in the parent function!
func deleteUser(ctx context.Context, tx *sql.Tx, userId string) error {
	_, err := tx.ExecContext(
		ctx,
		`DELETE FROM users WHERE id = $1`,
		userId,
	)
	if err != nil {
		return err
	}
	return nil
}