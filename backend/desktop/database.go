package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"
)

// func (context, current open transaction, user's ID, file hashes that we want to keep)
//   - deletes all files that aren't in the list of files that we want to keep
//   - assumes: you should also delete any associated processed data elsewhere
func batchDeleteFilepaths(ctx context.Context, tx *sql.Tx, userID string, toKeepHashes []string) error {
	_, err := tx.ExecContext(ctx, `
        DELETE FROM indexed_files
        WHERE user_id = $1 AND hash NOT IN (SELECT unnest($2::text[]))
    `, userID, pq.Array(toKeepHashes))
	if err != nil {
		return err
	}
	return nil
}

// func(context, current open transaction, user's ID, map of file name updates)
//   - applies file name changes for files whose content has not changed based on an updates[][] map
//   - assumes: files with the same hash are the same file (warning as file systems get larger this could be an issue)
func batchUpdateIndexedFiles(ctx context.Context, tx *sql.Tx, userID string, updates map[string]string) error {
	updateStmt, err := tx.PrepareContext(ctx, `
		UPDATE indexed_files
		SET file_path = $3
		WHERE user_id = $1 AND file_path = $2
		AND hash = (SELECT hash FROM indexed_files WHERE file_path = $3)
	`)
	if err != nil {
		return err
	}
	defer updateStmt.Close()

	// Execute batch updates
	for oldPath, newPath := range updates {
		_, err = updateStmt.ExecContext(ctx, userID, oldPath, newPath)
		if err != nil {
			return err
		}
	}

	return nil
}

// func(context, current open transaction, user's ID, list of new file paths, list of new file hashes_
//   - takes in a list of newfiles and hashes and inserts them into indexed_files for the userID
//   - assumes: newfiles and hashes have been processed already
func batchInsertFilepaths(ctx context.Context, tx *sql.Tx, userID string, newFiles []string, newHashes []string) error {
	_, err := tx.ExecContext(ctx, `
        INSERT INTO indexed_files (user_id, file_path, hash, done)
        SELECT $1, unnest($2::text[]), unnest($3::varchar[]), false
    `, userID, pq.Array(newFiles), pq.Array(newHashes))
	if err != nil {
		return fmt.Errorf("failed to execute insert query: %v", err)
	}
	return nil
}

// func(context, user's ID)
//   - takes in a userID and sets the starting total_files and crawled_files counts for the user
//   - assumes: that total_files and crawled_files are accurately set in indexed_files already
func (s *desktopServer) setStartingFileCount(ctx context.Context, tx *sql.Tx, userID string) error {
	var totalFiles, doneFiles int
	if err := tx.QueryRowContext(ctx, `
		SELECT
			COUNT(*) AS total_files,
			COUNT(NULLIF(done, FALSE)) AS done_files
		FROM
			indexed_files
		WHERE
			user_id = $1
	`, userID).Scan(&totalFiles, &doneFiles); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE crawl_stats
		SET
			total_files = $1,
			crawled_files = $2
		WHERE
			user_id = $3
	`, totalFiles, doneFiles, userID); err != nil {
		return err
	}

	return nil
}

// func(context, user's ID)
//   - creates a brand new crawl stats entry with default values for userID
//   - assumes: the user has been created elsewhere and is new
func (s *desktopServer) createDefaultCrawlStatsEntry(ctx context.Context, userID string) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO crawl_stats (user_id, online, crawling, crawled_files, total_files)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, false, false, 0, 0); err != nil {
		return err
	}
	return nil
}

// func(context, user's ID)
//   - returns if the user is crawling right now
//   - assumes: the crawling tag in the database is up-to-date and accurate
func (s *desktopServer) checkAndSetCrawling(ctx context.Context, userID string) (bool, error) {
	var isCrawling bool
	err := s.db.QueryRowContext(ctx, `
		UPDATE crawl_stats
		SET crawling = true
		WHERE user_id = $1 AND NOT crawling
		RETURNING crawling
	`, userID).Scan(&isCrawling)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}

	// this means that someone else was already crawling
	if err == sql.ErrNoRows {
		return true, nil
	}

	// otherwise no one was crawling before
	return false, nil
}

// func(context, current open transaction, user's ID, value of isCrawling to set)
//   - marks the user as crawling or not crawling based on isCrawling
func (s *desktopServer) markCrawling(ctx context.Context, tx *sql.Tx, userID string, isCrawling bool) error {
	// update crawling to done
	if _, err := tx.ExecContext(ctx, `
		UPDATE crawl_stats
		SET crawling = $1
		WHERE user_id = $2
	`, isCrawling, userID); err != nil {
		return err
	}
	return nil
}

// func(context, user's ID)
//   - marks the user as done with crawling
//   - assumes: the user's files are all done processing
func (s *desktopServer) markCrawlingDone(ctx context.Context, userID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// update the crawling to be done
	s.markCrawling(ctx, tx, userID, false)

	// commit the transaction
	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

// func(context, user's ID, file path to mark)
//   - marks the given filepath as being done with processing
//   - assumes: all chunks associated with the file are done processing
func (s *desktopServer) markFileDone(ctx context.Context, userID string, filePath string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// update file to done
	if _, err = tx.ExecContext(ctx, `
		UPDATE indexed_files
		SET done = true
		WHERE user_id = $1 AND file_path = $2
	`, userID, filePath); err != nil {
		return err
	}

	// update the counter in stats
	if _, err = tx.ExecContext(ctx, `
		UPDATE crawl_stats
		SET crawled_files = crawled_files + 1 
		WHERE user_id = $1
	`, userID); err != nil {
		return err
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// func(context, user's ID)
//   - gets a list of all the current files associated with userID
func (s *desktopServer) getCurrentFiles(ctx context.Context, userID string) (*sql.Rows, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT file_path, hash, done FROM indexed_files WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// func (context, time in seconds)
//   - sets all entries of crawl_stats to crawling = false if the current time is past the updated_at time by a certain amount
func (s *desktopServer) killIdleCrawls(ctx context.Context, allowedIdleTime time.Duration) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE crawl_stats
		SET crawling = false
		WHERE updated_at < NOW() - $1 * interval '1 second'
	`, allowedIdleTime.Seconds())
	return err
}

// func(context, current database pointer)
//   - sets all instances of crawling to false in crawl_stats
//   - assumes: the service is shutting down
func markAllCrawlingDone(ctx context.Context, db *sql.DB) {
	log.Print("killing all crawls by setting them to done!")
	if _, err := db.ExecContext(ctx, `
		UPDATE crawl_stats
		SET crawling = false
		WHERE crawling = true;
	`); err != nil {
		log.Print("failed to set all crawls to done")
	}
}

// func(context, user's ID)
//   - retrieves the crawl statistics for a given user
//   - returns crawled files count, total files count, crawling status, and online status
//   - assumes: the user exists in the database
func (s *desktopServer) getCrawlStats(ctx context.Context, userID string) (int32, int32, bool, bool, error) {
	var crawledFiles, totalFiles int32
	var isCrawling, isOnline bool

	err := s.db.QueryRowContext(ctx, `
		SELECT 
			crawled_files, 
			total_files, 
			crawling, 
			online
		FROM 
			crawl_stats
		WHERE 
			user_id = $1
	`, userID).Scan(&crawledFiles, &totalFiles, &isCrawling, &isOnline)

	if err != nil {
		return 0, 0, false, false, fmt.Errorf("failed to get crawl stats for user %s: %v", userID, err)
	}

	return crawledFiles, totalFiles, isCrawling, isOnline, nil
}

// func(context, user's ID, online status)
//   - updates the online status for a given user in the crawl_stats table
//   - returns an error if the update fails
//   - assumes: the user exists in the database
func (s *desktopServer) updateUserOnlineStatus(ctx context.Context, userID string, isOnline bool) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE crawl_stats
		SET 
			online = $2
		WHERE 
			user_id = $1
	`, userID, isOnline)

	if err != nil {
		return fmt.Errorf("failed to update online status for user %s: %v", userID, err)
	}

	return nil
}

// func(context, current open transaction, user's ID)
//   - deletes the crawl stats entry for a given user in the given transaction
//   - deletes all associated indexed_files for a given user in the given transaction
//   - assumes: the user exists in the database and you will close this transaction in the parent function
func (s *desktopServer) deleteUserStats(ctx context.Context, tx *sql.Tx, userID string) error {
	_, err := tx.ExecContext(ctx, `
		DELETE FROM crawl_stats
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user stats for user %s: %v", userID, err)
	}

	_, err = tx.ExecContext(ctx, `
		DELETE FROM indexed_files
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user files for user %s: %v", userID, err)
	}

	return nil
}