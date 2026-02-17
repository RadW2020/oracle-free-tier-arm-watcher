#!/bin/bash

# Cargar variables desde .env si existe
if [ -f .env ]; then
  export $(grep -v '^#' .env | xargs)
fi

# Verificar configuración
if [ -z "$CHECKLY_API_KEY" ] || [ -z "$CHECKLY_ACCOUNT_ID" ]; then
  echo "❌ Error: CHECKLY_API_KEY o CHECKLY_ACCOUNT_ID no están definidos en .env"
  exit 1
fi


# Obtener todos los checks
CHECKS=$(curl -s -X GET "https://api.checklyhq.com/v1/checks" \
  -H "Authorization: Bearer $CHECKLY_API_KEY" \
  -H "X-Checkly-Account: $CHECKLY_ACCOUNT_ID")

# Filtrar solo los que queremos (Monitor)
CHECK_IDS=$(echo "$CHECKS" | jq -r '.[] | select(.name | contains("Coolify") or contains("Oracle")) | .id')

echo "⏳ Cambiando frecuencia a 60 minutos (1 hora) para los checks detectados..."

for ID in $CHECK_IDS; do
  echo "Processing check $ID..."
  
  # Obtener el objeto completo del check
  CHECK_DATA=$(curl -s -X GET "https://api.checklyhq.com/v1/checks/$ID" \
    -H "Authorization: Bearer $CHECKLY_API_KEY" \
    -H "X-Checkly-Account: $CHECKLY_ACCOUNT_ID")
  
  # Cambiar la frecuencia a 60 en el JSON
  UPDATED_DATA=$(echo "$CHECK_DATA" | jq '.frequency = 60')
  
  # Hacer el PUT para actualizar
  RESPONSE=$(curl -s -X PUT "https://api.checklyhq.com/v1/checks/$ID" \
    -H "Authorization: Bearer $CHECKLY_API_KEY" \
    -H "X-Checkly-Account: $CHECKLY_ACCOUNT_ID" \
    -H "Content-Type: application/json" \
    -d "$UPDATED_DATA")
  
  if echo "$RESPONSE" | grep -q '"id"'; then
    NAME=$(echo "$RESPONSE" | jq -r '.name')
    echo "✅ '$NAME' actualizado a 60 min."
  else
    echo "❌ Error actualizando $ID:"
    echo "$RESPONSE"
  fi
  echo "-----------------------------------"
done
