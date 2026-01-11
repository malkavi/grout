import re
locales = ['es', 'fr', 'de', 'it', 'pt', 'ja', 'ru']
for lang in locales:
    filepath = f'resources/locales/active.{lang}.toml'
    with open(filepath, 'r') as f:
        content = f.read()
    entries = {}
    pattern = r'\[([^\]]+)\]\s*\n(?:hash\s*=\s*"[^"]*"\s*\n)?other\s*=\s*("(?:[^"\\]|\\.)*"|"""[\s\S]*?""")'
    for match in re.finditer(pattern, content):
        key, value = match.group(1), match.group(2)
        if value.startswith('"""'):
            inner = value[3:-3].strip('\n').replace('\n', '\\n').replace('"', '\\"')
            value = f'"{inner}"'
        entries[key] = value
    with open(filepath, 'w') as f:
        for key in sorted(entries.keys()):
            f.write(f'{key} = {entries[key]}\n')