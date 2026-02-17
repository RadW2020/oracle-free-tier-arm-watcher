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

CHECK_ID="c456ecb8-2004-4e2c-9444-ed8634b947f5"
APP_URL="http://tu-app.tu-dominio.com"
APP_API_KEY="$API_KEY"


echo "🔧 Actualizando check en Checkly..."

# Actualizar el check con umbral > 100%
RESPONSE=$(curl -s -X PUT "https://api.checklyhq.com/v1/checks/$CHECK_ID" \
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
          "property": "$.configured",
          "comparison": "EQUALS",
          "target": "true"
        },
        {
          "source": "JSON_BODY",
          "property": "$.maxUsagePercentage",
          "comparison": "LESS_THAN",
          "target": "101"
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
  echo "✅ Check actualizado exitosamente!"
  echo ""
  echo "Nueva configuración:"
  echo "  - ✅ Alerta si el servicio cae (status != 200)"
  echo "  - ✅ Alerta si NO está configurado"
  echo "  - 🔔 Alerta SOLO si uso > 100% (excede Free Tier)"
  echo ""
  echo "🟢 Al 100%: No alerta (uso óptimo del Free Tier)"
  echo "🔴 Al 101%+: ALERTA (empezarías a pagar)"
  echo ""
  echo "🔗 Ver en: https://app.checklyhq.com/checks/$CHECK_ID"
else
  echo "❌ Error actualizando el check:"
  echo "$RESPONSE" | jq '.' 2>/dev/null || echo "$RESPONSE"
  exit 1
fi
