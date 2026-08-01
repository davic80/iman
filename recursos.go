// Package iman expone las plantillas y los ficheros estaticos empotrados en el
// binario.
//
// Van empotrados a proposito: asi la imagen final no necesita copiar
// directorios ni preocuparse de rutas relativas, y el contenedor puede ser un
// distroless con un unico fichero dentro. Si el binario arranca, tiene todo lo
// que necesita.
package iman

import "embed"

//go:embed plantillas/*.html
var Plantillas embed.FS

//go:embed estaticos
var Estaticos embed.FS
