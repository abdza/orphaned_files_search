package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/microsoft/go-mssqldb"
	_ "modernc.org/sqlite"
)

type FileInfo struct {
	Path         string
	Size         int64
	LastModified time.Time
	TableName    string
	RecordID     int
	Module       string
}

type TreeReport struct {
	ID           int
	RootLocation string
}

type Setting struct {
	ID   int
	Name string
	Text string
}

func normalizePath(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.ReplaceAll(path, "//", "/")
	return path
}

func parseRootLocation(rootLocation string) string {
	parts := strings.Split(rootLocation, "${")
	parsed := normalizePath(parts[0])
	if len(parsed) > 5 {
		return parsed
	}
	return ""
}

func main() {
	rootFolder := flag.String("root", "", "Root folder to search")
	sqlServer := flag.String("server", "", "MS SQL Server address")
	port := flag.Int("port", 1433, "MS SQL Server port")
	username := flag.String("username", "", "MS SQL Server username")
	password := flag.String("password", "", "MS SQL Server password")
	database := flag.String("database", "", "MS SQL Server database name")
	verbose := flag.Bool("verbose", false, "Enable verbose output")
	flag.Parse()

	if *rootFolder == "" || *sqlServer == "" || *username == "" || *password == "" || *database == "" {
		log.Fatal("All parameters are required except port (default is 1433)")
	}

	// Connect to MS SQL Server
	connString := fmt.Sprintf("server=%s;port=%d;user id=%s;password=%s;database=%s", *sqlServer, *port, *username, *password, *database)
	mssqlDB, err := sql.Open("sqlserver", connString)
	if err != nil {
		log.Fatalf("Error connecting to MS SQL Server: %v", err)
	}
	defer mssqlDB.Close()

	// Create SQLite database
	sqliteDB, err := sql.Open("sqlite", "file_search_results.db")
	if err != nil {
		log.Fatalf("Error creating SQLite database: %v", err)
	}
	defer sqliteDB.Close()

	// Create table in SQLite
	_, err = sqliteDB.Exec(`
		CREATE TABLE IF NOT EXISTS file_search_results (
			path TEXT PRIMARY KEY,
			size INTEGER,
			last_modified DATETIME,
			table_name TEXT,
			record_id INTEGER,
			module TEXT,
			is_orphaned BOOLEAN
		)
	`)
	if err != nil {
		log.Fatalf("Error creating table in SQLite: %v", err)
	}

	// Prepare SQLite statements
	insertOrUpdate, err := sqliteDB.Prepare(`
		INSERT INTO file_search_results (path, size, last_modified, table_name, record_id, module, is_orphaned)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
		size = excluded.size,
		last_modified = excluded.last_modified,
		table_name = excluded.table_name,
		record_id = excluded.record_id,
		module = excluded.module,
		is_orphaned = excluded.is_orphaned
	`)
	if err != nil {
		log.Fatalf("Error preparing SQLite statement: %v", err)
	}
	defer insertOrUpdate.Close()

	// Fetch tree_report data
	treeReports, err := fetchTreeReports(mssqlDB)
	if err != nil {
		log.Fatalf("Error fetching tree reports: %v", err)
	}

	// Fetch settings data
	settings, err := fetchSettings(mssqlDB)
	if err != nil {
		log.Fatalf("Error fetching settings: %v", err)
	}

	if *verbose {
		fmt.Printf("Loaded %d valid tree reports and %d settings\n", len(treeReports), len(settings))
	}

	fileCount := 0
	orphanedCount := 0

	// Walk through the files
	err = filepath.Walk(*rootFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			fileCount++
			normalizedPath := normalizePath(path)
			fileInfo := FileInfo{
				Path:         normalizedPath,
				Size:         info.Size(),
				LastModified: info.ModTime(),
			}

			if *verbose {
				fmt.Printf("Processing file: %s\n", normalizedPath)
			}

			// Check if file exists in MS SQL Server
			var recordID int
			var module sql.NullString
			err := mssqlDB.QueryRow(`
				SELECT id, module 
				FROM file_link 
				WHERE REPLACE(REPLACE(path, '\', '/'), '//', '/') = @p1
			`, normalizedPath).Scan(&recordID, &module)
			
			if err == sql.ErrNoRows {
				// File is not in file_link table, check tree_report
				treeReportID := findMatchingTreeReport(normalizedPath, treeReports)
				if treeReportID != 0 {
					fileInfo.TableName = "tree_report"
					fileInfo.RecordID = treeReportID
					if *verbose {
						fmt.Printf("File matched tree_report: %s (Report ID: %d)\n", normalizedPath, treeReportID)
					}
				} else {
					// Check settings table
					settingID, settingName := findMatchingSetting(normalizedPath, settings)
					if settingID != 0 {
						fileInfo.TableName = "settings"
						fileInfo.RecordID = settingID
						fileInfo.Module = settingName
						if *verbose {
							fmt.Printf("File matched settings: %s (Setting ID: %d, Name: %s)\n", normalizedPath, settingID, settingName)
						}
					} else {
						// File is truly orphaned
						orphanedCount++
						if *verbose {
							fmt.Printf("Orphaned file found: %s\n", normalizedPath)
						}
					}
				}
			} else if err != nil {
				log.Printf("Error querying MS SQL Server: %v", err)
			} else {
				// File is found in the file_link table
				fileInfo.TableName = "file_link"
				fileInfo.RecordID = recordID
				if module.Valid {
					fileInfo.Module = module.String
				}
				if *verbose {
					fmt.Printf("File found in file_link: %s (ID: %d, Module: %s)\n", normalizedPath, recordID, fileInfo.Module)
				}
			}

			_, err = insertOrUpdate.Exec(fileInfo.Path, fileInfo.Size, fileInfo.LastModified, fileInfo.TableName, fileInfo.RecordID, fileInfo.Module, fileInfo.TableName == "")
			if err != nil {
				log.Printf("Error inserting/updating file in SQLite: %v", err)
			}
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Error walking through files: %v", err)
	}

	fmt.Printf("File search completed. Processed %d files, found %d orphaned files. Results stored in file_search_results.db\n", fileCount, orphanedCount)
}

func findMatchingTreeReport(filePath string, treeReports []TreeReport) int {
	for _, tr := range treeReports {
		if strings.HasPrefix(strings.ToLower(filePath), strings.ToLower(tr.RootLocation)) {
			return tr.ID
		}
	}
	return 0
}

func fetchTreeReports(db *sql.DB) ([]TreeReport, error) {
	rows, err := db.Query(`SELECT id, REPLACE(REPLACE(rootlocation, '\', '/'), '//', '/') as rootlocation FROM tree_report`)
	if err != nil {
		return nil, fmt.Errorf("error querying tree_report table: %v", err)
	}
	defer rows.Close()

	var treeReports []TreeReport
	for rows.Next() {
		var tr TreeReport
		if err := rows.Scan(&tr.ID, &tr.RootLocation); err != nil {
			log.Printf("Error scanning tree_report row: %v", err)
			continue
		}
		if parsedRoot := parseRootLocation(tr.RootLocation); parsedRoot != "" {
			tr.RootLocation = parsedRoot
			treeReports = append(treeReports, tr)
		}
	}
	return treeReports, nil
}

func fetchSettings(db *sql.DB) ([]Setting, error) {
	rows, err := db.Query(`
		SELECT id, name, REPLACE(REPLACE(cast(text as nvarchar(max)), '\', '/'), '//', '/') as text 
		FROM settings 
		WHERE text LIKE '%csdportal%' 
		AND name NOT LIKE '%path%' 
		AND name != 'uploadfolder' 
		AND text NOT LIKE 'http:%' 
		AND text NOT LIKE 'jdbc:%' 
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("error querying settings table: %v", err)
	}
	defer rows.Close()

	var settings []Setting
	for rows.Next() {
		var s Setting
		if err := rows.Scan(&s.ID, &s.Name, &s.Text); err != nil {
			log.Printf("Error scanning settings row: %v", err)
			continue
		}
		s.Text = parseRootLocation(s.Text)
		if s.Text != "" {
			settings = append(settings, s)
		}
	}
	return settings, nil
}

func findMatchingSetting(filePath string, settings []Setting) (int, string) {
	for _, s := range settings {
		if strings.HasPrefix(strings.ToLower(filePath), strings.ToLower(s.Text)) {
			return s.ID, s.Name
		}
	}
	return 0, ""
}
