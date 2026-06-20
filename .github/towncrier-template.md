{% if sections[''] %}
{% if sections['']['summary'] %}

{% for text, _ in sections['']['summary'].items() %}
{{ text }}
{% endfor %}
{% endif %}
{% for category, definition in definitions.items() if category != 'summary' %}
{% if sections[''][category] %}

### {{ definition.name }}

{% for text, values in sections[''][category].items() %}
- {{ text }}{% if values %} ({{ values|join(', ') }}){% endif +%}
{% endfor %}
{% endif %}
{% endfor %}

{% else %}

No significant changes.

{% endif %}
