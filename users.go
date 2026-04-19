package main

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"os"
	"time"
)

const keyChars = "abcdefghijklmnopqrstuvwxyz0123456789"
const keyLength = 8

var adjectives = []string{
	"Agile", "Amber", "Azure", "Bold", "Brave",
	"Bright", "Calm", "Chill", "Clean", "Clear",
	"Clever", "Cloudy", "Cool", "Cosmic", "Crafty",
	"Crisp", "Crystal", "Daring", "Dazzle", "Deft",
	"Deep", "Dreamy", "Dusk", "Eager", "Earthy",
	"Ember", "Epic", "Fair", "Fancy", "Fierce",
	"Fine", "Fleet", "Fresh", "Frost", "Gentle",
	"Gleam", "Glow", "Golden", "Grand", "Happy",
	"Hardy", "Honest", "Humble", "Indigo", "Ivory",
	"Jade", "Jolly", "Joyful", "Keen", "Kind",
	"Lilac", "Lucky", "Luminous", "Mellow", "Merry",
	"Mighty", "Neat", "Noble", "Olive", "Opal",
	"Peaceful", "Pearl", "Peppy", "Placid", "Proud",
	"Pure", "Quick", "Quiet", "Rare", "Rapid",
	"Ready", "Rosy", "Ruby", "Rustic", "Sage",
	"Sharp", "Shiny", "Silent", "Silky", "Silver",
	"Slate", "Smooth", "Snowy", "Solid", "Spicy",
	"Stark", "Steady", "Stern", "Stout", "Strong",
	"Sunny", "Swift", "Teal", "Tender", "Thick",
	"Tiny", "True", "Vast", "Vivid", "Warm",
	"Wild", "Willow", "Wise", "Witty", "Zen",
}

var animals = []string{
	"Ant", "Ape", "Badger", "Bat", "Bear",
	"Bee", "Bird", "Bison", "Boar", "Buck",
	"Bull", "Camel", "Cat", "Cobra", "Cod",
	"Colt", "Condor", "Coyote", "Crab", "Crane",
	"Crow", "Cub", "Deer", "Dodo", "Dove",
	"Dragon", "Duck", "Eagle", "Eel", "Elk",
	"Emu", "Falcon", "Finch", "Fish", "Flea",
	"Fly", "Fox", "Frog", "Gecko", "Gibbon",
	"Gnu", "Goat", "Goose", "Grizzly", "Gull",
	"Hamster", "Hare", "Hawk", "Heron", "Hippo",
	"Hog", "Horse", "Hyena", "Ibis", "Jackal",
	"Jaguar", "Jay", "Jellyfish", "Kestrel", "Kit",
	"Koala", "Koi", "Ladybug", "Lamb", "Lark",
	"Lemur", "Leopard", "Lion", "Lizard", "Llama",
	"Lynx", "Mantis", "Mare", "Mink", "Mole",
	"Mongoose", "Moose", "Moth", "Mule", "Newt",
	"Octopus", "Okapi", "Opossum", "Orca", "Otter",
	"Owl", "Panda", "Panther", "Parrot", "Peacock",
	"Pelican", "Penguin", "Pheasant", "Pig", "Pike",
	"Pony", "Prawn", "Pug", "Puma", "Python",
	"Quail", "Rabbit", "Raccoon", "Ram", "Raven",
	"Ray", "Rhea", "Robin", "Rook", "Sable",
	"Salmon", "Seal", "Shark", "Sheep", "Shrimp",
	"Sloth", "Snail", "Snake", "Sparrow", "Spider",
	"Sponge", "Squid", "Stork", "Swan", "Tapir",
	"Termite", "Tiger", "Toad", "Trout", "Tuna",
	"Turkey", "Turtle", "Viper", "Vole", "Wasp",
	"Whale", "Wolf", "Wolverine", "Wombat", "Wren",
	"Yak", "Zebra",
}

func generateRandomKey() (string, error) {
	key := make([]byte, keyLength)
	for i := range key {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(keyChars))))
		if err != nil {
			return "", fmt.Errorf("generate key: %w", err)
		}
		key[i] = keyChars[n.Int64()]
	}
	return string(key), nil
}

func generateRandomName() (string, error) {
	adjIdx, err := rand.Int(rand.Reader, big.NewInt(int64(len(adjectives))))
	if err != nil {
		return "", fmt.Errorf("generate name: %w", err)
	}
	animalIdx, err := rand.Int(rand.Reader, big.NewInt(int64(len(animals))))
	if err != nil {
		return "", fmt.Errorf("generate name: %w", err)
	}
	return adjectives[adjIdx.Int64()] + animals[animalIdx.Int64()], nil
}

func createUser(db *sql.DB, name, key string) (*User, error) {
	now := time.Now().Unix()
	result, err := db.Exec(
		"INSERT INTO users (name, key, created_at) VALUES (?, ?, ?)",
		name, key, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create user %q: %w", name, err)
	}
	id, _ := result.LastInsertId()
	return &User{ID: id, Name: name, Key: key, CreatedAt: now}, nil
}

func getUserByName(db *sql.DB, name string) (*User, error) {
	var u User
	err := db.QueryRow("SELECT id, name, key, created_at FROM users WHERE name = ?", name).
		Scan(&u.ID, &u.Name, &u.Key, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func validateUser(db *sql.DB, name, key string) (*User, error) {
	user, err := getUserByName(db, name)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user %q not found", name)
	}
	if user.Key != key {
		return nil, fmt.Errorf("invalid key for user %q", name)
	}
	return user, nil
}

func generateUniqueName(db *sql.DB) (string, error) {
	for range 10 {
		name, err := generateRandomName()
		if err != nil {
			return "", err
		}
		existing, err := getUserByName(db, name)
		if err != nil {
			return "", err
		}
		if existing == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("could not generate a unique name after 10 attempts")
}

func getSession() (name, key string) {
	return os.Getenv("JOBS_USER"), os.Getenv("JOBS_KEY")
}

func requireAuth(db *sql.DB) (*User, error) {
	name, key := getSession()
	if name == "" || key == "" {
		return nil, fmt.Errorf("not logged in. Run 'eval $(jobs login)' to get started")
	}
	user, err := validateUser(db, name, key)
	if err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}
	return user, nil
}

type LoginResult struct {
	Name  string
	Key   string
	IsNew bool
}

func runLogin(db *sql.DB, name, key string) (*LoginResult, error) {
	if name == "" {
		generatedName, err := generateUniqueName(db)
		if err != nil {
			return nil, err
		}
		generatedKey, err := generateRandomKey()
		if err != nil {
			return nil, err
		}
		_, err = createUser(db, generatedName, generatedKey)
		if err != nil {
			return nil, err
		}
		return &LoginResult{Name: generatedName, Key: generatedKey, IsNew: true}, nil
	}

	existing, err := getUserByName(db, name)
	if err != nil {
		return nil, err
	}

	if existing == nil {
		if key == "" {
			key, err = generateRandomKey()
			if err != nil {
				return nil, err
			}
		}
		_, err = createUser(db, name, key)
		if err != nil {
			return nil, err
		}
		return &LoginResult{Name: name, Key: key, IsNew: true}, nil
	}

	if key == "" {
		return nil, fmt.Errorf("user %q already exists. Provide the key: job login %s <key>", name, name)
	}

	user, err := validateUser(db, name, key)
	if err != nil {
		return nil, err
	}
	return &LoginResult{Name: user.Name, Key: user.Key, IsNew: false}, nil
}

func formatLoginExport(name, key string) string {
	return fmt.Sprintf("export JOBS_USER=%q JOBS_KEY=%q", name, key)
}

func formatLogoutExport() string {
	return "unset JOBS_USER JOBS_KEY"
}
