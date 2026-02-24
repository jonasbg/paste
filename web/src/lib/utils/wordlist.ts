// Wordlist for client-side passphrase generation.
// Mirror of crypto/wordlist.go — keep both in sync when adding words.
// With 4 words + 4-char alphanumeric suffix: ~59.0 bits of entropy (log2(760^4 * 36^4))
const WORDLIST: readonly string[] = [
	'able', 'acid', 'aged', 'also', 'area', 'army', 'away', 'baby',
	'back', 'ball', 'band', 'bank', 'base', 'bath', 'bear', 'beat',
	'been', 'beer', 'bell', 'belt', 'best', 'bill', 'bird', 'blow',
	'blue', 'boat', 'body', 'bomb', 'bond', 'bone', 'book', 'boom',
	'born', 'boss', 'both', 'bowl', 'bulk', 'burn', 'bush', 'busy',
	'cafe', 'cake', 'call', 'calm', 'came', 'camp', 'card', 'care',
	'case', 'cash', 'cast', 'cell', 'chat', 'chef', 'chip', 'city',
	'clay', 'clip', 'club', 'coal', 'coat', 'code', 'coin', 'cold',
	'come', 'cook', 'cool', 'cope', 'copy', 'core', 'corn', 'cost',
	'coup', 'crew', 'crop', 'cult', 'cure', 'dare', 'dark', 'data',
	'date', 'dawn', 'days', 'dead', 'deaf', 'deal', 'dean', 'dear',
	'debt', 'deck', 'deep', 'deny', 'desk', 'dial', 'diet', 'disc',
	'dock', 'does', 'done', 'door', 'dose', 'down', 'draw', 'drew',
	'drop', 'drug', 'dual', 'duck', 'duke', 'dull', 'duly', 'dump',
	'dust', 'duty', 'each', 'earl', 'earn', 'ease', 'east', 'easy',
	'echo', 'edge', 'edit', 'else', 'even', 'ever', 'evil', 'exit',
	'face', 'fact', 'fail', 'fair', 'fall', 'fame', 'fare', 'farm',
	'fast', 'fate', 'fear', 'feat', 'feel', 'feet', 'fell', 'felt',
	'file', 'fill', 'film', 'find', 'fine', 'fire', 'firm', 'fish',
	'five', 'flag', 'flat', 'fled', 'flew', 'flow', 'folk', 'food',
	'fool', 'foot', 'ford', 'form', 'fort', 'four', 'free', 'from',
	'fuel', 'full', 'fund', 'gain', 'game', 'gate', 'gave', 'gear',
	'gene', 'gift', 'girl', 'give', 'glad', 'goal', 'goes', 'gold',
	'golf', 'gone', 'good', 'gray', 'grew', 'grey', 'grow', 'gulf',
	'hair', 'half', 'hall', 'hand', 'hang', 'hard', 'harm', 'hate',
	'have', 'head', 'hear', 'heat', 'held', 'hell', 'help', 'here',
	'hero', 'hide', 'high', 'hill', 'hint', 'hire', 'hold', 'hole',
	'holy', 'home', 'hope', 'host', 'hour', 'huge', 'hung', 'hunt',
	'hurt', 'icon', 'idea', 'inch', 'into', 'iron', 'item', 'jail',
	'jean', 'join', 'joke', 'jump', 'jury', 'just', 'keen', 'keep',
	'kent', 'kept', 'kick', 'kill', 'kind', 'king', 'kiss', 'knee',
	'knew', 'know', 'lack', 'lady', 'laid', 'lake', 'land', 'lane',
	'last', 'late', 'lead', 'leaf', 'lean', 'left', 'less', 'lest',
	'life', 'lift', 'like', 'line', 'link', 'list', 'live', 'load',
	'loan', 'lock', 'long', 'look', 'loop', 'lord', 'lose', 'loss',
	'lost', 'love', 'luck', 'lung', 'made', 'mail', 'main', 'make',
	'male', 'mall', 'many', 'mark', 'mass', 'mate', 'math', 'meal',
	'mean', 'meat', 'meet', 'menu', 'mere', 'mess', 'mice', 'mile',
	'milk', 'mill', 'mind', 'mine', 'miss', 'mode', 'mood', 'moon',
	'more', 'most', 'move', 'much', 'must', 'myth', 'name', 'navy',
	'near', 'neck', 'need', 'news', 'next', 'nice', 'nine', 'none',
	'noon', 'norm', 'nose', 'note', 'nude', 'oath', 'obey', 'ocean',
	'odd', 'okay', 'once', 'only', 'onto', 'open', 'oral', 'oven',
	'over', 'pace', 'pack', 'page', 'paid', 'pain', 'pair', 'pale',
	'palm', 'park', 'part', 'pass', 'past', 'path', 'peak', 'peer',
	'pick', 'pile', 'pill', 'pine', 'pink', 'pipe', 'plan', 'play',
	'plot', 'plug', 'plus', 'poem', 'poet', 'pole', 'poll', 'pond',
	'pool', 'poor', 'pope', 'port', 'pose', 'post', 'pour', 'pray',
	'prep', 'prey', 'pull', 'pure', 'push', 'quad', 'quit', 'quiz',
	'race', 'rail', 'rain', 'rank', 'rare', 'rate', 'read', 'real',
	'rear', 'rely', 'rent', 'rest', 'rice', 'rich', 'ride', 'ring',
	'rise', 'risk', 'road', 'rock', 'role', 'roll', 'rome', 'roof',
	'room', 'root', 'rope', 'rose', 'ruby', 'rule', 'rush', 'rust',
	'safe', 'sage', 'said', 'sake', 'sale', 'salt', 'same', 'sand',
	'save', 'says', 'scan', 'seat', 'sect', 'seed', 'seek', 'seem',
	'seen', 'self', 'sell', 'send', 'sent', 'sept', 'ship',
	'shop', 'shot', 'show', 'shut', 'sick', 'side', 'sign', 'silk',
	'sing', 'sink', 'site', 'size', 'skin', 'skip', 'slim', 'slip',
	'slot', 'slow', 'snow', 'soft', 'soil', 'sold', 'sole', 'some',
	'song', 'soon', 'sort', 'soul', 'spot', 'star', 'stay', 'stem',
	'step', 'stir', 'stop', 'such', 'suit', 'sure', 'take', 'tale',
	'talk', 'tall', 'tank', 'tape', 'task', 'team', 'tear', 'tech',
	'tell', 'tend', 'term', 'test', 'text', 'than', 'that', 'thee',
	'them', 'then', 'they', 'thin', 'this', 'thus', 'tide', 'tied',
	'till', 'time', 'tiny', 'tips', 'tone', 'took', 'tool', 'tops',
	'torn', 'tour', 'town', 'tree', 'trek', 'trim', 'trio', 'trip',
	'true', 'tube', 'tune', 'turn', 'twin', 'type', 'unit', 'upon',
	'used', 'user', 'vary', 'vast', 'very', 'vice', 'view', 'vote',
	'wage', 'wait', 'wake', 'walk', 'wall', 'want', 'ward', 'warm',
	'warn', 'wash', 'wave', 'ways', 'weak', 'wear', 'week', 'well',
	'went', 'were', 'west', 'what', 'when', 'whom', 'wide', 'wife',
	'wild', 'will', 'wind', 'wine', 'wing', 'wire', 'wise', 'wish',
	'with', 'wood', 'word', 'wore', 'work', 'worn', 'wrap', 'yard',
	'yeah', 'year', 'your', 'zero', 'zone', 'acre', 'aide', 'aims',
	'ajar', 'ally', 'amid', 'aqua', 'arch', 'atom', 'aunt', 'axis',
	'heal', 'herb', 'yoga', 'zest', 'mend', 'tonic', 'fiber', 'fresh',
	'sleep', 'water', 'pulse', 'organ', 'heart', 'brain', 'nurse', 'medic',
	'clean', 'flora', 'smile', 'happy', 'focus', 'zen', 'jonas', 'august',
	'paris', 'berlin', 'madrid', 'lisbon', 'vienna', 'prague', 'zurich', 'geneva',
	'oslo', 'athens', 'dublin', 'warsaw', 'naples', 'turin', 'sevilla', 'malaga',
	'porto', 'brno', 'riga', 'vilnius', 'tallinn', 'sofia', 'zagreb', 'skopje',
	'tirana', 'bologna', 'venice', 'milan', 'bergen', 'tromso', 'drammen', 'bodo',
	'larvik', 'hamar', 'molde', 'alta', 'narvik', 'skien', 'svalbard', 'longyear',
	'kirkenes', 'arendal', 'sandnes', 'tonsberg', 'notodden', 'leknes', 'halden',
	'january', 'february', 'march', 'april', 'may', 'june', 'july', 'september',
	'october', 'november', 'december',
	'river', 'forest', 'meadow', 'cedar', 'maple', 'birch', 'willow', 'stone',
	'cloud', 'frost', 'breeze', 'summit', 'valley', 'harbor', 'island', 'canyon',
	'garden', 'cabin', 'bridge', 'lantern', 'window', 'kettle', 'anchor', 'button',
	'pocket', 'candle', 'mirror', 'basket', 'hammer', 'saddle', 'compass', 'blanket',
	'ribbon', 'tunnel', 'wander', 'gather', 'build', 'carry', 'weave', 'drift',
	'sprint', 'climb', 'sketch', 'craft', 'forge', 'travel', 'apple', 'berry',
	'olive', 'mango', 'cocoa', 'basil', 'honey', 'lemon', 'peach', 'walnut',
	'barley', 'bright', 'steady', 'gentle', 'noble', 'brave', 'loyal', 'vivid',
	'sunny', 'mellow', 'radiant', 'humble', 'kindly', 'serene', 'lively', 'placid',
	'crisp', 'sacred', 'worthy', 'honest', 'grin', 'laugh', 'smiley',
];

const ALPHANUMERIC = 'abcdefghijklmnopqrstuvwxyz0123456789';
const DIGITS = '0123456789';

function generateSuffix(length = 4): string {
	const rnd = crypto.getRandomValues(new Uint32Array(length));
	const chars = Array.from(rnd, (n) => ALPHANUMERIC[n % ALPHANUMERIC.length]);

	// Ensure at least one digit
	const hasDigit = chars.some((c) => c >= '0' && c <= '9');
	if (!hasDigit) {
		const [posRnd, digitRnd] = crypto.getRandomValues(new Uint32Array(2));
		chars[posRnd % length] = DIGITS[digitRnd % DIGITS.length];
	}

	return chars.join('');
}

/**
 * Generate a random passphrase entirely in the browser using crypto.getRandomValues().
 * Format: word-word-word-word-x7k3
 * Matches the format produced by crypto.GeneratePassphrase() in Go.
 */
export function generatePassphrase(numWords = 4): string {
	const indices = crypto.getRandomValues(new Uint32Array(numWords));
	const words = Array.from(indices, (n) => WORDLIST[n % WORDLIST.length]);
	return [...words, generateSuffix()].join('-');
}
