package cliutils

import (
	"fmt"
	"os"

	"github.com/codaxa/tunnelR.git/cmd/cli/shared"
	"github.com/olekukonko/tablewriter"
)

// PrintMachinesTable to print machines in a table
func PrintMachinesTable(machines []shared.Machine) {
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"ID", "HOSTNAME", "IP ADDRESS", "CREATED AT"})
	for _, m := range machines {
		if err := table.Append([]string{m.ID, m.Hostname, m.IPAddress, m.CreatedAt.Format("Jan 02, 2006 15:04")}); err != nil {
			fmt.Printf("Error appending at the table: %s", err)
		}
	}
	if err := table.Render(); err != nil {
		fmt.Printf("Error rendering the table: %s", err)
	}
	fmt.Printf("\nTotal: %d machines\n", len(machines))
}

// PrintUsersTable to print users in a table
func PrintUsersTable(users []shared.User) {
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"ID", "Username", "CREATED AT"})
	for _, u := range users {
		if err := table.Append([]string{u.ID, u.Username, u.CreatedAt.Format("Jan 02, 2006 15:04")}); err != nil {
			fmt.Printf("Error appending at the table: %s", err)
		}
	}
	if err := table.Render(); err != nil {
		fmt.Printf("Error rendering the table: %s", err)
	}
	fmt.Printf("\nTotal: %d users\n", len(users))
}

// PrintTeamsTable to print teams in a table
func PrintTeamsTable(teams []shared.Team) {
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"ID", "Name", "CREATED AT"})
	for _, t := range teams {
		if err := table.Append([]string{t.ID, t.Name, t.CreatedAt.Format("Jan 02, 2006 15:04")}); err != nil {
			fmt.Printf("Error appending at the table: %s", err)
		}
	}
	if err := table.Render(); err != nil {
		fmt.Printf("Error rendering the table: %s", err)
	}
	fmt.Printf("\nTotal: %d teams\n", len(teams))
}
