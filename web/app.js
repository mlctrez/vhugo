'use strict';

// dependencies
var deps = ['ngRoute', 'ngMaterial', 'ngWebSocket'];

// application module
var vhugo = angular.module('vhugo', deps);

// allows use of href="javascript:void(0)" without angular displaying
// an unsafe: warning in the footer menu
vhugo.config(['$compileProvider', function ($compileProvider) {
    $compileProvider.aHrefSanitizationWhitelist(/^\s*(https?|ftp|mailto|file|javascript):/);
}]);

// dialog buttons don't show up good, but it is better on the eyes at night
vhugo.config(function ($mdThemingProvider) {
    $mdThemingProvider.theme('default').dark();
});

vhugo.config(['$httpProvider', function ($httpProvider) {
    $httpProvider.interceptors.push('responseErrorInterceptor');
}]);

vhugo.factory('responseErrorInterceptor', ['$q', function ($q) {
    return {
        responseError: function (response) {
            console.log(response.data);
            return $q.reject(response);
        }
    };
}]);


// routes
vhugo.config(['$routeProvider', function ($rp) {
    $rp.when('/home', {
        templateUrl: 'home.html',
        controller: 'HomeController'
    }).when('/setup', {
        templateUrl: 'setup.html',
        controller: 'SetupController'
    }).otherwise({
        redirectTo: '/home'
    });
}]);


vhugo.controller('HomeController', function ($scope, $http, $mdDialog, $websocket) {
    $scope.lights = [];

    // TODO: move to factory
    var loc = window.location, new_uri;
    if (loc.protocol === "https:") {
        new_uri = "wss:";
    } else {
        new_uri = "ws:";
    }
    new_uri += "//" + loc.host;
    new_uri += "/messages";

    $scope.ws = $websocket(new_uri, {"reconnectIfNotNormalClose": true});

    $scope.ws.onOpen(function (message) {
        console.log("websocket onOpen " + message);
    });
    $scope.ws.onClose(function (message) {
        console.log("websocket onClose " + message);
    });
    $scope.ws.onError(function (message) {
        console.log("websocket onError " + message);
    });

    $scope.ws.onMessage(function (message) {

        var d = JSON.parse(message.data);

        // find light and update it
        $scope.lights.forEach(function (value) {
            if (d.groupID !== value.group_id) {
                return
            }
            if (d.lightID !== value.light_id) {
                return
            }
            if (d.stateRequest.on !== null) {
                value.on = d.stateRequest.on;
            }
            if (d.stateRequest.bri !== null) {
                value.brightness = d.stateRequest.bri;
            }
        });
    });

    $scope.changeState = function (l) {
        var lurl = '/api/lights/' + l.group_id + '/' + l.light_id;
        $http.post(lurl, {"on": l.on});
    };

    $scope.changeBrightness = function (l) {
        var lurl = '/api/lights/' + l.group_id + '/' + l.light_id;
        $http.post(lurl, {"bri": l.brightness});
    };

    $scope.queryLights = function () {
        $http.get('/api/lights').success(function (data) {
            $scope.lights = data.lights;
        });
    };

    $scope.addLight = function (ev) {
        var confirm = $mdDialog.prompt()
            .title('Add Light')
            .textContent('What is the name of the light?')
            .placeholder('Name')
            .ariaLabel('Name')
            .initialValue('')
            .targetEvent(ev)
            .ok('Ok')
            .cancel('cancel');

        $mdDialog.show(confirm).then(function (result) {
            console.log("confirm " + result);
            $http.post('/api/lights', {"name": result}).success(function (data) {
                $scope.queryLights();
            });
        }, function () {
            console.log("addLight cancel");
        });
    };

    $scope.deleteLight = function (ev, light) {
        var confirm = $mdDialog.confirm()
            .title('Would you like to delete ' + light.name + '?')
            .textContent('This operation is irreversable.')
            .ariaLabel('Delete light')
            .targetEvent(ev)
            .ok('Ok')
            .cancel('Cancel');

        $mdDialog.show(confirm).then(function () {
            var lurl = '/api/lights/' + light.group_id + '/' + light.light_id;
            $http.delete(lurl).success(function (data) {
                $scope.queryLights();
            });
        }, function () {
            console.log("deleteLight cancel");
        });
    };

    $scope.queryLights();

});