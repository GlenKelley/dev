package git

import "os/exec"
import "strings"

func GitRoot() (string, error) {
    bytes, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
    if err != nil {
        groot := strings.TrimSpace(string(bytes))
        return groot, nil
    } 
    return "", err
}
