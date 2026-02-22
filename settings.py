TEMPLATES = [
    {
        'BACKEND': 'django.template.backends.jinja2.Jinja2',
        'DIRS': [
            "web/templates"
        ],
        'APP_DIRS': True,
        'OPTIONS': { },
    },
    {
        'BACKEND': 'django.template.backends.django.DjangoTemplates',
        'DIRS': [
            "web/templates"
        ],
        'APP_DIRS': True,
    },
]


