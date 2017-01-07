package vhugo

import (
	"text/template"
)

var discoveryResponseText = `HTTP/1.1 200 OK
CACHE-CONTROL: max-age=86400
EXT:
LOCATION: http://{{.ServerIP}}:{{.ServerPort}}/upnp/{{.GroupID}}/setup.xml
OPT: "http://schemas.upnp.org/upnp/1/0/"; ns=01
01-NLS: {{.UUID}}
ST: urn:schemas-upnp-org:device:basic:1
USN: uuid:Socket-1_0-{{.UU}}::urn:Belkin:device:**

`
var disoveryResponseTemplate = template.Must(template.New("discoveryResponse").Parse(discoveryResponseText))

var settingsText = `<?xml version="1.0"?><root xmlns="urn:schemas-upnp-org:device-1-0">
<specVersion><major>1</major><minor>0</minor></specVersion>
<URLBase>http://{{.ServerIP}}:{{.ServerPort}}/</URLBase>
<device>
<deviceType>urn:schemas-upnp-org:device:Basic:1</deviceType>
<friendlyName>VHugo {{.UU}}</friendlyName>
<manufacturer>Royal Philips Electronics</manufacturer>
<manufacturerURL>https://github.com/mlctrez</manufacturerURL>
<modelDescription>Hue Go Emulator</modelDescription>
<modelName>Philips hue bridge 2012</modelName>
<modelNumber>929000226503</modelNumber>
<modelURL>https://github.com/mlctrez/vhugo</modelURL>
<serialNumber>{{.UU}}</serialNumber>
<UDN>uuid:{{.UUID}}</UDN>
</device>
</root>
`
var settingsTemplate = template.Must(template.New("settings").Parse(settingsText))
