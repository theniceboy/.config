# Defined in - @ line 1
function gc --description 'alias gc git config credential.helper store'
	git config credential.helper store $argv;
end
