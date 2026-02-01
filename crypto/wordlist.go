package crypto

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

// Wordlist contains 512 memorable words for passphrase generation
// Chosen to be short (3-8 chars), memorable, and phonetically distinct
var Wordlist = []string{
	"able", "acid", "aged", "also", "area", "army", "away", "baby",
	"back", "ball", "band", "bank", "base", "bath", "bear", "beat",
	"been", "beer", "bell", "belt", "best", "bill", "bird", "blow",
	"blue", "boat", "body", "bomb", "bond", "bone", "book", "boom",
	"born", "boss", "both", "bowl", "bulk", "burn", "bush", "busy",
	"cafe", "cake", "call", "calm", "came", "camp", "card", "care",
	"case", "cash", "cast", "cell", "chat", "chef", "chip", "city",
	"clay", "clip", "club", "coal", "coat", "code", "coin", "cold",
	"come", "cook", "cool", "cope", "copy", "core", "corn", "cost",
	"coup", "crew", "crop", "cult", "cure", "dare", "dark", "data",
	"date", "dawn", "days", "dead", "deaf", "deal", "dean", "dear",
	"debt", "deck", "deep", "deny", "desk", "dial", "diet", "disc",
	"dock", "does", "done", "door", "dose", "down", "draw", "drew",
	"drop", "drug", "dual", "duck", "duke", "dull", "duly", "dump",
	"dust", "duty", "each", "earl", "earn", "ease", "east", "easy",
	"echo", "edge", "edit", "else", "even", "ever", "evil", "exit",
	"face", "fact", "fail", "fair", "fall", "fame", "fare", "farm",
	"fast", "fate", "fear", "feat", "feel", "feet", "fell", "felt",
	"file", "fill", "film", "find", "fine", "fire", "firm", "fish",
	"five", "flag", "flat", "fled", "flew", "flow", "folk", "food",
	"fool", "foot", "ford", "form", "fort", "four", "free", "from",
	"fuel", "full", "fund", "gain", "game", "gate", "gave", "gear",
	"gene", "gift", "girl", "give", "glad", "goal", "goes", "gold",
	"golf", "gone", "good", "gray", "grew", "grey", "grow", "gulf",
	"hair", "half", "hall", "hand", "hang", "hard", "harm", "hate",
	"have", "head", "hear", "heat", "held", "hell", "help", "here",
	"hero", "hide", "high", "hill", "hint", "hire", "hold", "hole",
	"holy", "home", "hope", "host", "hour", "huge", "hung", "hunt",
	"hurt", "icon", "idea", "inch", "into", "iron", "item", "jail",
	"jean", "join", "joke", "jump", "jury", "just", "keen", "keep",
	"kent", "kept", "kick", "kill", "kind", "king", "kiss", "knee",
	"knew", "know", "lack", "lady", "laid", "lake", "land", "lane",
	"last", "late", "lead", "leaf", "lean", "left", "less", "lest",
	"life", "lift", "like", "line", "link", "list", "live", "load",
	"loan", "lock", "long", "look", "loop", "lord", "lose", "loss",
	"lost", "love", "luck", "lung", "made", "mail", "main", "make",
	"male", "mall", "many", "mark", "mass", "mate", "math", "meal",
	"mean", "meat", "meet", "menu", "mere", "mess", "mice", "mile",
	"milk", "mill", "mind", "mine", "miss", "mode", "mood", "moon",
	"more", "most", "move", "much", "must", "myth", "name", "navy",
	"near", "neck", "need", "news", "next", "nice", "nine", "none",
	"noon", "norm", "nose", "note", "nude", "oath", "obey", "ocean",
	"odd", "okay", "once", "only", "onto", "open", "oral", "oven",
	"over", "pace", "pack", "page", "paid", "pain", "pair", "pale",
	"palm", "park", "part", "pass", "past", "path", "peak", "peer",
	"pick", "pile", "pill", "pine", "pink", "pipe", "plan", "play",
	"plot", "plug", "plus", "poem", "poet", "pole", "poll", "pond",
	"pool", "poor", "pope", "port", "pose", "post", "pour", "pray",
	"prep", "prey", "pull", "pure", "push", "quad", "quit", "quiz",
	"race", "rail", "rain", "rank", "rare", "rate", "read", "real",
	"rear", "rely", "rent", "rest", "rice", "rich", "ride", "ring",
	"rise", "risk", "road", "rock", "role", "roll", "rome", "roof",
	"room", "root", "rope", "rose", "ruby", "rule", "rush", "rust",
	"safe", "sage", "said", "sake", "sale", "salt", "same", "sand",
	"save", "says", "scan", "seat", "sect", "seed", "seek", "seem",
	"seen", "self", "sell", "send", "sens", "sent", "sept", "ship",
	"shop", "shot", "show", "shut", "sick", "side", "sign", "silk",
	"sing", "sink", "site", "size", "skin", "skip", "slim", "slip",
	"slot", "slow", "snow", "soft", "soil", "sold", "sole", "some",
	"song", "soon", "sort", "soul", "spot", "star", "stay", "stem",
	"step", "stir", "stop", "such", "suit", "sure", "take", "tale",
	"talk", "tall", "tank", "tape", "task", "team", "tear", "tech",
	"tell", "tend", "term", "test", "text", "than", "that", "thee",
	"them", "then", "they", "thin", "this", "thus", "tide", "tied",
	"till", "time", "tiny", "tips", "tone", "took", "tool", "tops",
	"torn", "tour", "town", "tree", "trek", "trim", "trio", "trip",
	"true", "tube", "tune", "turn", "twin", "type", "unit", "upon",
	"used", "user", "vary", "vast", "very", "vice", "view", "vote",
	"wage", "wait", "wake", "walk", "wall", "want", "ward", "warm",
	"warn", "wash", "wave", "ways", "weak", "wear", "week", "well",
	"went", "were", "west", "what", "when", "whom", "wide", "wife",
	"wild", "will", "wind", "wine", "wing", "wire", "wise", "wish",
	"with", "wood", "word", "wore", "work", "worn", "wrap", "yard",
	"yeah", "year", "your", "zero", "zone",
}

// GeneratePassphrase generates a random passphrase with the specified number of words
func GeneratePassphrase(numWords int) (string, error) {
	if numWords < 2 || numWords > 10 {
		return "", fmt.Errorf("number of words must be between 2 and 10")
	}

	words := make([]string, numWords)
	max := big.NewInt(int64(len(Wordlist)))

	for i := 0; i < numWords; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}
		words[i] = Wordlist[n.Int64()]
	}

	return strings.Join(words, "-"), nil
}

// ValidatePassphrase validates that a passphrase consists of valid words
func ValidatePassphrase(passphrase string) error {
	words := strings.Split(passphrase, "-")
	if len(words) < 2 || len(words) > 10 {
		return fmt.Errorf("passphrase must contain 2-10 words")
	}

	// Build wordlist lookup map
	validWords := make(map[string]bool, len(Wordlist))
	for _, word := range Wordlist {
		validWords[word] = true
	}

	for _, word := range words {
		if !validWords[word] {
			return fmt.Errorf("invalid word in passphrase: %s", word)
		}
	}

	return nil
}

// GetWordCompletions returns possible completions for a partial word
func GetWordCompletions(prefix string) []string {
	var completions []string
	prefix = strings.ToLower(prefix)

	for _, word := range Wordlist {
		if strings.HasPrefix(word, prefix) {
			completions = append(completions, word)
		}
	}

	return completions
}
