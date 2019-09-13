# Defined in - @ line 1
function fzf --description alias\ fzf\ fzf\ --preview\ \'head\ -100\ \{\}\'
	command fzf --preview 'head -100 {}' $argv;
end
