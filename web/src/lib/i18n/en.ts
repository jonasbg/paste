const en = {
	common: {
		retry: 'Try again',
		cancel: 'Cancel',
		or: 'or',
		copy: 'Copy',
		copied: 'Copied',
		error: 'Error',
		download: 'Download',
		loadingShort: 'Loading...',
		remaining: 'left'
	},
	home: {
		titleBefore: 'We ',
		titleLink: 'share',
		titleAfter: ' files securely',
		description:
			"Share files securely with end-to-end encryption. Files are encrypted in your browser before they're uploaded, and only decrypted when the recipient downloads them."
	},
	upload: {
		dropzoneAria: 'Choose a file to upload',
		dropPrimary: 'Click here to choose a file — or drag and drop to upload',
		maxFileSize: 'Maximum file size {size}',
		uploadNow: 'Upload',
		removeFile: 'Remove file',
		fileTooLarge: 'The file is too large. Maximum file size is {size}.'
	},
	paste: {
		reading: 'Reading clipboard...',
		denied: 'Access denied',
		empty: 'Clipboard is empty',
		idle: 'Paste from clipboard'
	},
	passphrase: {
		hint: 'Enter the sharing code you received to download the file.',
		placeholder: 'Enter your sharing code',
		findFile: 'Find file',
		invalidCode: "Invalid sharing code or the file doesn't exist. Try again."
	},
	preview: {
		title: 'Preview',
		image: 'Image',
		text: 'Text',
		loadingImage: 'Loading image preview...',
		loadingText: 'Loading text preview...',
		copyAria: 'Copy text',
		emptyTextFile: 'This text file is empty.',
		truncated: 'The preview is truncated. Download the file to see the full content.',
		imageAlt: 'Preview of {filename}',
		textOnlyLimit: 'Preview is only available for text files up to {limit}.',
		textLoadError: 'Could not load the text file preview.',
		imageOnlyLimit: 'Preview is only available for image files up to {limit}.',
		imageLoadError: 'Could not load the image file preview.'
	},
	download: {
		completeTitle: 'Download complete',
		downloadingAria: 'Downloading...',
		fileDeleted: 'The file has been deleted from the server.'
	},
	dl: {
		titleLink: 'Secure',
		titleAfter: ' file sharing',
		description:
			'Welcome to our secure file sharing service. Here you can safely download files that have been shared with you. All files are end-to-end encrypted. After a successful download, the file is automatically deleted from our servers.',
		metadataError:
			"Could not fetch file information. Check that the key is correct, or that the file hasn't been deleted.",
		decryptError: 'Could not decrypt the file - the file has now been deleted from the server',
		deleteNetworkError:
			'The file was downloaded, but could not be deleted from the server due to a network error.'
	},
	key: {
		requiredTitle: 'Decryption key required',
		hint: 'Paste the full link you received and the key will be extracted automatically. Alternatively, you can paste the decryption key directly.',
		placeholder: 'Paste the decryption key or the full URL',
		decrypt: 'Decrypt',
		invalid: 'Invalid key or URL'
	},
	loading: {
		fetchingInfo: 'Fetching file information...'
	},
	share: {
		viaCode: 'Share via code',
		easierBadge: 'Easier to share',
		codeHint:
			'A readable code the recipient types in themselves. Slightly lower entropy than a random key.',
		link: 'Link',
		codeOnly: 'Code only',
		viaSecureLink: 'Share via secure link',
		higherSecurityBadge: 'Higher security',
		secureHint:
			"Random cryptographic key in the URL. Can't be memorized — share the whole link at once.",
		completeLink: 'Complete link',
		webAddress: 'Web address',
		key: 'Key',
		labelCode: 'Sharing code',
		copiedToast: '{label} copied!'
	},
	service: {
		uploading: 'Uploading...',
		done: 'Done!',
		uploadNetworkError: 'Network error during upload',
		uploadUnknownError: 'Unknown upload error',
		connectionAborted: 'The connection was unexpectedly interrupted',
		connectionClosed: 'The connection was closed',
		startingDownload: 'Starting download...',
		downloading: 'Downloading...',
		downloadComplete: 'Download complete',
		metadataFetchError: 'Could not fetch file information',
		fileNotFound: "The file doesn't exist or has expired"
	}
};

export default en;
