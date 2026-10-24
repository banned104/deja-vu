package main

import (
	"os"
	"strings"
	"testing"
)

// --no-guidance writes no /deja recipe, and install said it had created one
// anyway: the goose-auto line named deja-recipe.yaml whatever the flag was
// (#5589). With the flag the line is gone and so is the file; without it both
// are there.
func TestGooseAutoNoGuidanceDoesNotNameTheRecipe(t *testing.T) {
	hermeticEnv(t)
	out, errOut := captureBoth(t, "install", "goose-auto", "--no-guidance", "--no-index")
	if strings.Contains(out+errOut, "deja-recipe.yaml") {
		t.Errorf("--no-guidance names a recipe it did not write:\n%s%s", out, errOut)
	}
	if _, err := os.Stat(gooseRecipePath()); !os.IsNotExist(err) {
		t.Errorf("--no-guidance wrote the recipe: %v", err)
	}

	hermeticEnv(t)
	out, errOut = captureBoth(t, "install", "goose-auto", "--no-index")
	if !strings.Contains(out+errOut, "also created "+shortHome(gooseRecipePath())) {
		t.Errorf("install no longer names the recipe it wrote:\n%s%s", out, errOut)
	}
	if _, err := os.Stat(gooseRecipePath()); err != nil {
		t.Errorf("install did not write the recipe: %v", err)
	}
}
