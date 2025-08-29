package utils

import (
	"fmt"
	"os"

	"github.com/codaxa/tunnelR.git/cmd/cli/shared"
	"github.com/olekukonko/tablewriter"
)

func PrintMachinesTable(machines []shared.Machine) {
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"ID", "HOSTNAME", "IP ADDRESS", "CREATED AT"})
	for _, m := range machines {
		table.Append([]string{m.ID, m.Hostname, m.IPAddress, m.CreatedAt.Format("Jan 02, 2006 15:04")})
	}
	table.Render()
	fmt.Printf("\nTotal: %d machines\n", len(machines))
}

func PrintUsersTable(users []shared.User) {
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"ID", "Username", "CREATED AT"})
	for _, u := range users {
		table.Append([]string{u.ID, u.Username, u.CreatedAt.Format("Jan 02, 2006 15:04")})
	}
	table.Render()
	fmt.Printf("\nTotal: %d users\n", len(users))
}

func PrintTeamssTable(teams []shared.Team) {
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"ID", "Name", "CREATED AT"})
	for _, t := range teams {
		table.Append([]string{t.ID, t.Name, t.CreatedAt.Format("Jan 02, 2006 15:04")})
	}
	table.Render()
	fmt.Printf("\nTotal: %d teams\n", len(teams))
}
