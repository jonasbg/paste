const no = {
	common: {
		retry: 'Prøv igjen',
		cancel: 'Avbryt',
		or: 'eller',
		copy: 'Kopier',
		copied: 'Kopiert',
		error: 'Feil',
		download: 'Last ned',
		loadingShort: 'Laster...',
		remaining: 'igjen'
	},
	home: {
		titleBefore: 'Vi ',
		titleLink: 'deler',
		titleAfter: ' filer sikkert',
		description:
			'Del filer sikkert med ende-til-ende-kryptering. Filene krypteres i nettleseren din før de lastes opp, og dekrypteres først når mottakeren laster dem ned.'
	},
	upload: {
		dropzoneAria: 'Velg fil for opplasting',
		dropPrimary: 'Klikk her for å velge fil — eller dra og slipp for å laste opp',
		maxFileSize: 'Maksimum filstørrelse {size}',
		uploadNow: 'Last opp',
		removeFile: 'Fjern fil',
		fileTooLarge: 'Filen er for stor. Maksimal filstørrelse er {size}.'
	},
	paste: {
		reading: 'Leser utklippstavlen...',
		denied: 'Tilgang nektet',
		empty: 'Utklippstavlen er tom',
		idle: 'Lim inn fra utklippstavlen'
	},
	passphrase: {
		hint: 'Skriv inn delingskoden du har mottatt for å laste ned filen.',
		placeholder: 'Skriv inn delingskoden din',
		findFile: 'Finn fil',
		invalidCode: 'Ugyldig delingskode eller filen finnes ikke. Prøv igjen.'
	},
	preview: {
		title: 'Forhåndsvisning',
		image: 'Bilde',
		text: 'Tekst',
		loadingImage: 'Laster bildeforhåndsvisning...',
		loadingText: 'Laster tekstforhåndsvisning...',
		copyAria: 'Kopier tekst',
		emptyTextFile: 'Denne tekstfilen er tom.',
		truncated: 'Forhåndsvisningen er avkortet. Last ned filen for å se hele innholdet.',
		imageAlt: 'Forhåndsvisning av {filename}',
		textOnlyLimit: 'Forhåndsvisning er bare tilgjengelig for tekstfiler opptil {limit}.',
		textLoadError: 'Kunne ikke laste forhåndsvisning av tekstfilen.',
		imageOnlyLimit: 'Forhåndsvisning er bare tilgjengelig for bildefiler opptil {limit}.',
		imageLoadError: 'Kunne ikke laste forhåndsvisning av bildefilen.'
	},
	download: {
		completeTitle: 'Nedlasting fullført',
		downloadingAria: 'Laster ned...',
		fileDeleted: 'Filen er slettet fra serveren.'
	},
	dl: {
		titleLink: 'Sikker',
		titleAfter: ' fildeling',
		description:
			'Velkommen til vår sikre fildelingstjeneste. Her kan du trygt laste ned filer som har blitt delt med deg. Alle filer er ende-til-ende-kryptert. Etter vellykket nedlasting blir filen automatisk slettet fra våre servere.',
		metadataError:
			'Kunne ikke hente filinformasjon. Sjekk at nøkkelen er riktig, eller at filen ikke er slettet.',
		decryptError: 'Kunne ikke dekryptere filen - filen er nå slettet fra serveren',
		deleteNetworkError:
			'Filen ble lastet ned, men kunne ikke slettes fra serveren på grunn av en nettverksfeil.'
	},
	key: {
		requiredTitle: 'Dekrypteringsnøkkel kreves',
		hint: 'Lim inn hele lenken du har mottatt, så vil nøkkelen automatisk bli hentet ut. Alternativt kan du lime inn dekrypteringsnøkkelen direkte.',
		placeholder: 'Lim inn dekrypteringsnøkkel eller hele URL-en',
		decrypt: 'Dekrypter',
		invalid: 'Ugyldig nøkkel eller URL'
	},
	loading: {
		fetchingInfo: 'Henter filinformasjon...'
	},
	share: {
		viaCode: 'Del via delingskode',
		easierBadge: 'Enklere å dele',
		codeHint:
			'En lesbar kode mottakeren skriver inn selv. Litt lavere entropi enn en tilfeldig nøkkel.',
		link: 'Lenke',
		codeOnly: 'Kun kode',
		viaSecureLink: 'Del via sikker lenke',
		higherSecurityBadge: 'Høyere sikkerhet',
		secureHint:
			'Tilfeldig kryptografisk nøkkel i URL-en. Kan ikke huskes — del hele lenken på én gang.',
		completeLink: 'Komplett lenke',
		webAddress: 'Nettadresse',
		key: 'Nøkkel',
		labelCode: 'Delingskode',
		copiedToast: '{label} kopiert!'
	},
	service: {
		uploading: 'Laster opp...',
		done: 'Ferdig!',
		uploadNetworkError: 'Nettverksfeil under opplasting',
		uploadUnknownError: 'Ukjent opplastingsfeil',
		connectionAborted: 'Tilkoblingen ble uventet avbrutt',
		connectionClosed: 'Tilkoblingen ble lukket',
		startingDownload: 'Starter nedlasting...',
		downloading: 'Laster ned...',
		downloadComplete: 'Nedlasting fullført',
		metadataFetchError: 'Kunne ikke hente filinformasjon',
		fileNotFound: 'Filen finnes ikke eller har utløpt'
	}
};

export default no;
