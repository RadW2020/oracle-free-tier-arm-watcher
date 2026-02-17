#!/bin/bash

# Cargar variables desde .env si existe
if [ -f .env ]; then
  export $(grep -v '^#' .env | xargs)
fi

# Verificar configuración básica
if [ -z "$CHECKLY_API_KEY" ] || [ -z "$CHECKLY_ACCOUNT_ID" ] || [ -z "$API_KEY" ]; then
  echo "❌ Error: CHECKLY_API_KEY, CHECKLY_ACCOUNT_ID o API_KEY no están definidos en .env"
  exit 1
fi

APP_URL="http://tu-app.tu-dominio.com"
APP_API_KEY="$API_KEY"


echo "🚀 Creando check en Checkly..."

# Crear el check
RESPONSE=$(curl -s -X POST "https://api.checklyhq.com/v1/checks" \
  -H "Authorization: Bearer $CHECKLY_API_KEY" \
  -H "X-Checkly-Account: $CHECKLY_ACCOUNT_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Oracle Free Tier Monitor",
    "checkType": "API",
    "frequency": 720,
    "activated": true,
    "muted": false,
    "doubleCheck": true,
    "shouldFail": false,
    "locations": ["eu-central-1"],
    "request": {
      "method": "GET",
      "url": "'$APP_URL'/status",
      "headers": [
        {
          "key": "X-API-Key",
          "value": "'$APP_API_KEY'",
          "locked": false
        }
      ],
      "assertions": [
        {
          "source": "STATUS_CODE",
          "comparison": "EQUALS",
          "target": "200"
        },
        {
          "source": "JSON_BODY",
          "property": "$.status",
          "comparison": "EQUALS",
          "target": "OK"
        },
        {
          "source": "JSON_BODY",
          "property": "$.maxUsagePercentage",
          "comparison": "LESS_THAN",
          "target": "80"
        }
      ]
    },
    "retryStrategy": {
      "type": "FIXED",
      "baseBackoffSeconds": 30,
      "maxRetries": 2,
      "maxDurationSeconds": 600,
      "sameRegion": true
    }
  }')

# Verificar respuesta
if echo "$RESPONSE" | grep -q '"id"'; then
  CHECK_ID=$(echo "$RESPONSE" | grep -o '"id":"[^"]*' | head -1 | sed 's/"id":"//')
  echo "✅ Check creado exitosamente!"
  echo "📊 Check ID: $CHECK_ID"
  echo "🔗 Ver en: https://app.checklyhq.com/checks/$CHECK_ID"
else
  echo "❌ Error creando el check:"
  echo "$RESPONSE" | jq '.' 2>/dev/null || echo "$RESPONSE"
  exit 1
fi

echo ""
echo "🎉 Configuración completada!"
echo ""
echo "El check se ejecutará cada 12 horas y te alertará si:"
echo "  - El servicio deja de funcionar"
echo "  - El uso supera el 80%"
echo ""
echo "Próximos pasos:"
echo "  1. Ve a Checkly → Alert Settings para configurar Email/Telegram"
echo "  2. Haz un 'Run Check' manual para verificar que funciona"
