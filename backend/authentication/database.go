package main

import (
	"context"
	"database/sql"
)

/**************
** USER CRUD **
***************/

// func(context, current open transaction, email, password hash, name, alias)
//   - inserts a user into the users table with the given email, password hash, and name in the given transaction
//   - returns: the new user's ID or an error
//   - assumes: you will close this transaction in the parent function!
func createUser(ctx context.Context, tx *sql.Tx, email string, passwordHash string, name string) (string, error) {
	var userId string	
	err := tx.QueryRowContext(
		ctx,
		"INSERT INTO users (email, password_hash, name, alias) VALUES ($1, $2, $3, $4) RETURNING id",
		email,
		passwordHash,
		name,
		name,
	).Scan(&userId)
	if err != nil {
		return "", err
	}
	return userId, nil
}

// func(context, current open transaction, user ID)
//   - fetches a user's email, password hash, name, and alias from the users table with the given user ID in the given transaction
//   - returns: email, password hash, name, alias, or an error
//   - assumes: you will close this transaction in the parent function!
func getUserById(ctx context.Context, tx *sql.Tx, userId string) (string, string, string, string, error) {
	var email, passwordHash, name, alias string
	err := tx.QueryRowContext(
		ctx,
		"SELECT email, password_hash, name, alias FROM users WHERE id = $1",
		userId,
	).Scan(&email, &passwordHash, &name, &alias)
	if err != nil {
		return "", "", "", "", err
	}
	return email, passwordHash, name, alias, nil
}

// func(context, current open transaction, email)
//   - fetches a user's id, password hash, name, and alias from the users table with the given email in the given transaction
//   - returns: user ID, password hash, name, alias, or an error
//   - assumes: you will close this transaction in the parent function!
func getUserByEmail(ctx context.Context, tx *sql.Tx, email string) (string, string, string, string, error) {
	var userId, passwordHash, name, alias string
	err := tx.QueryRowContext(
		ctx,
		"SELECT id, password_hash, name, alias FROM users WHERE email = $1",
		email,
	).Scan(&userId, &passwordHash, &name, &alias)
	if err != nil {
		return "", "", "", "", err
	}
	return userId, passwordHash, name, alias, nil
}

// func(context, current open transaction, user ID, email, password hash, name, alias)
//   - updates a user's email, password hash, name, and alias in the users table with the given user ID in the given transaction
//   - returns: the updated user's ID or an error
//   - assumes: you will close this transaction in the parent function!
func updateUser(ctx context.Context, tx *sql.Tx, userId string, email string, passwordHash string, name string, alias string) (string, error) {
	_, err := tx.ExecContext(
		ctx,
		`UPDATE users SET email = $1, password_hash = $2, name = $3, alias = $4 WHERE id = $5`,
		email,
		passwordHash,
		name,
		alias,
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