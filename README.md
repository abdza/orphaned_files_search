# Orphaned Files Search Program

## Overview

The Orphaned Files Search Program is a Go-based utility designed to identify and catalog files within a specified directory structure. It cross-references these files against entries in an MS SQL Server database, specifically looking at the `file_link` and `tree_report` tables. The program categorizes files as either associated with database entries or orphaned, storing the results in a SQLite database for easy access and analysis.

## Features

- Recursive file system traversal from a specified root directory
- Integration with MS SQL Server for database queries
- Comparison against `file_link` and `tree_report` tables
- Handling of complex path patterns in the `tree_report` table
- SQLite database output for search results
- Verbose mode for detailed operation logging

## Prerequisites

- Go 1.15 or higher
- Access to an MS SQL Server database
- SQLite support

## Installation

1. Clone the repository or download the source code.

2. Install the required Go packages:

   ```
   go get github.com/microsoft/go-mssqldb
   go get github.com/mattn/go-sqlite3
   ```

3. Build the program:

   ```
   go build -o orphaned-files-search
   ```

## Usage

Run the program with the following command-line arguments:

```
./orphaned-files-search -root <root_folder> -server <sql_server> -username <username> -password <password> -database <database_name> [-verbose]
```

### Parameters:

- `-root`: The root folder to start the file search
- `-server`: MS SQL Server address
- `-username`: MS SQL Server username
- `-password`: MS SQL Server password
- `-database`: MS SQL Server database name
- `-verbose`: (Optional) Enable verbose output

### Example:

```
./orphaned-files-search -root /path/to/files -server sqlserver.example.com -username myuser -password mypass -database mydb -verbose
```

## Output

The program generates a SQLite database file named `file_search_results.db` in the current directory. This database contains a table `file_search_results` with the following columns:

- `path`: The full path of the file
- `size`: File size in bytes
- `last_modified`: Last modification timestamp
- `table_name`: Either 'file_link' or 'tree_report', indicating which table the file was found in
- `record_id`: The ID of the matching record in the respective table
- `module`: Module information (only for files found in 'file_link')
- `is_orphaned`: Boolean indicating whether the file is orphaned

## Database Schema

The program expects the following tables in the MS SQL Server database:

1. `file_link` table:
   - `id`: Unique identifier
   - `path`: File path
   - `module`: Module information

2. `tree_report` table:
   - `id`: Unique identifier
   - `rootlocation`: Root location path (may include parameters)

## Notes

- The program considers a file "orphaned" if it's not found in either the `file_link` table or doesn't match any valid `rootlocation` in the `tree_report` table.
- Root locations in the `tree_report` table must have more than 5 characters to be considered valid.
- The program handles parameterized paths in the `tree_report.rootlocation` field by truncating at the first occurrence of "${".

## Troubleshooting

- Ensure you have the necessary permissions to access the MS SQL Server database and the file system.
- Check your MS SQL Server connection string if you encounter database connection issues.
- Make sure the `file_link` and `tree_report` tables exist in your database with the expected schema.

## Contributing

Contributions to improve the Orphaned Files Search Program are welcome. Please feel free to submit pull requests or open issues to discuss proposed changes or report bugs.

## License

[Specify your license here, e.g., MIT, GPL, etc.]

